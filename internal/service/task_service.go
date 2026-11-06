package service

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"task-manager-api/internal/repository"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

var validStatuses = map[string]bool{
	"todo":        true,
	"in_progress": true,
	"completed":   true,
	"cancelled":   true,
}

var validPriorities = map[string]bool{
	"low":    true,
	"medium": true,
	"high":   true,
	"urgent": true,
}

// CreateTaskInput holds payload for creating a task.
type CreateTaskInput struct {
	ProjectID   *uuid.UUID  `json:"project_id"`
	AssigneeID  *uuid.UUID  `json:"assignee_id"`
	Title       string      `json:"title"`
	Description string      `json:"description"`
	Status      string      `json:"status"`
	Priority    string      `json:"priority"`
	DueDate     *time.Time  `json:"due_date"`
	TagIDs      []uuid.UUID `json:"tag_ids"`
}

// UpdateTaskInput holds payload for updating a task.
type UpdateTaskInput struct {
	ProjectID   *uuid.UUID  `json:"project_id"`
	AssigneeID  *uuid.UUID  `json:"assignee_id"`
	Title       string      `json:"title"`
	Description string      `json:"description"`
	Status      string      `json:"status"`
	Priority    string      `json:"priority"`
	DueDate     *time.Time  `json:"due_date"`
	Version     int32       `json:"version"`
	TagIDs      []uuid.UUID `json:"tag_ids"`
}

// ListTasksFilter holds criteria for filtering tasks.
type ListTasksFilter struct {
	ProjectID *uuid.UUID `json:"project_id"`
	Status    *string    `json:"status"`
	Priority  *string    `json:"priority"`
	Page      int32      `json:"page"`
	Limit     int32      `json:"limit"`
}

// TaskResponse represents public task details with tags.
type TaskResponse struct {
	ID          uuid.UUID     `json:"id"`
	ProjectID   *uuid.UUID    `json:"project_id,omitempty"`
	CreatorID   uuid.UUID     `json:"creator_id"`
	AssigneeID  *uuid.UUID    `json:"assignee_id,omitempty"`
	Title       string        `json:"title"`
	Description string        `json:"description"`
	Status      string        `json:"status"`
	Priority    string        `json:"priority"`
	DueDate     *time.Time    `json:"due_date,omitempty"`
	Version     int32         `json:"version"`
	Tags        []TagResponse `json:"tags"`
	CreatedAt   time.Time     `json:"created_at"`
	UpdatedAt   time.Time     `json:"updated_at"`
}

// PaginatedTasksResponse represents paginated task results.
type PaginatedTasksResponse struct {
	Items      []TaskResponse `json:"items"`
	TotalItems int64          `json:"total_items"`
	Page       int32          `json:"page"`
	Limit      int32          `json:"limit"`
	TotalPages int32          `json:"total_pages"`
}

// TaskService defines business logic for tasks.
type TaskService interface {
	CreateTask(ctx context.Context, userID uuid.UUID, input CreateTaskInput) (*TaskResponse, error)
	GetTaskByID(ctx context.Context, userID uuid.UUID, taskID uuid.UUID) (*TaskResponse, error)
	ListTasks(ctx context.Context, userID uuid.UUID, filter ListTasksFilter) (*PaginatedTasksResponse, error)
	UpdateTask(ctx context.Context, userID uuid.UUID, taskID uuid.UUID, input UpdateTaskInput) (*TaskResponse, error)
	DeleteTask(ctx context.Context, userID uuid.UUID, taskID uuid.UUID) error
}

type taskService struct {
	repo repository.Querier
}

// NewTaskService constructs a new TaskService.
func NewTaskService(repo repository.Querier) TaskService {
	return &taskService{repo: repo}
}

// CreateTask validates inputs, verifies project ownership, and creates task + tags.
func (s *taskService) CreateTask(ctx context.Context, userID uuid.UUID, input CreateTaskInput) (*TaskResponse, error) {
	status := strings.ToLower(strings.TrimSpace(input.Status))
	if status == "" {
		status = "todo"
	} else if !validStatuses[status] {
		return nil, ErrInvalidTaskStatus
	}

	priority := strings.ToLower(strings.TrimSpace(input.Priority))
	if priority == "" {
		priority = "medium"
	} else if !validPriorities[priority] {
		return nil, ErrInvalidTaskPriority
	}

	var pID pgtype.UUID
	if input.ProjectID != nil {
		project, err := s.repo.GetProjectByID(ctx, *input.ProjectID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, ErrProjectNotFound
			}
			return nil, fmt.Errorf("failed to verify project: %w", err)
		}
		if project.OwnerID != userID {
			return nil, ErrForbidden
		}
		pID = pgtype.UUID{Bytes: *input.ProjectID, Valid: true}
	}

	var aID pgtype.UUID
	if input.AssigneeID != nil {
		aID = pgtype.UUID{Bytes: *input.AssigneeID, Valid: true}
	}

	var dDate pgtype.Timestamptz
	if input.DueDate != nil {
		dDate = pgtype.Timestamptz{Time: *input.DueDate, Valid: true}
	}

	task, err := s.repo.CreateTask(ctx, repository.CreateTaskParams{
		ProjectID:   pID,
		CreatorID:   userID,
		AssigneeID:  aID,
		Title:       strings.TrimSpace(input.Title),
		Description: strings.TrimSpace(input.Description),
		Status:      status,
		Priority:    priority,
		DueDate:     dDate,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to persist task: %w", err)
	}

	// Attach tags if provided
	for _, tagID := range input.TagIDs {
		_ = s.repo.AddTagToTask(ctx, repository.AddTagToTaskParams{
			TaskID: task.ID,
			TagID:  tagID,
		})
	}

	tags, _ := s.repo.GetTagsForTask(ctx, task.ID)
	return s.mapTask(task, tags), nil
}

// GetTaskByID retrieves task and its tags, verifying access permissions.
func (s *taskService) GetTaskByID(ctx context.Context, userID uuid.UUID, taskID uuid.UUID) (*TaskResponse, error) {
	task, err := s.repo.GetTaskByID(ctx, taskID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrTaskNotFound
		}
		return nil, fmt.Errorf("failed to get task: %w", err)
	}

	// Verify permission: creator or assignee
	hasAccess := task.CreatorID == userID || (task.AssigneeID.Valid && task.AssigneeID.Bytes == userID)
	if !hasAccess && task.ProjectID.Valid {
		project, err := s.repo.GetProjectByID(ctx, task.ProjectID.Bytes)
		if err == nil && project.OwnerID == userID {
			hasAccess = true
		}
	}

	if !hasAccess {
		return nil, ErrForbidden
	}

	tags, err := s.repo.GetTagsForTask(ctx, task.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to get task tags: %w", err)
	}

	return s.mapTask(task, tags), nil
}

// ListTasks retrieves paginated tasks filtered by project, status, priority.
func (s *taskService) ListTasks(ctx context.Context, userID uuid.UUID, filter ListTasksFilter) (*PaginatedTasksResponse, error) {
	page := filter.Page
	if page <= 0 {
		page = 1
	}

	limit := filter.Limit
	if limit <= 0 {
		limit = 10
	} else if limit > 100 {
		limit = 100
	}

	offset := (page - 1) * limit

	var pID pgtype.UUID
	if filter.ProjectID != nil {
		pID = pgtype.UUID{Bytes: *filter.ProjectID, Valid: true}
	}

	var status pgtype.Text
	if filter.Status != nil && *filter.Status != "" {
		st := strings.ToLower(strings.TrimSpace(*filter.Status))
		if !validStatuses[st] {
			return nil, ErrInvalidTaskStatus
		}
		status = pgtype.Text{String: st, Valid: true}
	}

	var priority pgtype.Text
	if filter.Priority != nil && *filter.Priority != "" {
		pr := strings.ToLower(strings.TrimSpace(*filter.Priority))
		if !validPriorities[pr] {
			return nil, ErrInvalidTaskPriority
		}
		priority = pgtype.Text{String: pr, Valid: true}
	}

	total, err := s.repo.CountTasks(ctx, repository.CountTasksParams{
		CreatorID: userID,
		ProjectID: pID,
		Status:    status,
		Priority:  priority,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to count tasks: %w", err)
	}

	tasks, err := s.repo.ListTasks(ctx, repository.ListTasksParams{
		CreatorID: userID,
		Limit:     limit,
		Offset:    offset,
		ProjectID: pID,
		Status:    status,
		Priority:  priority,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list tasks: %w", err)
	}

	// Fetch tags in batch
	taskIDs := make([]uuid.UUID, len(tasks))
	for i, t := range tasks {
		taskIDs[i] = t.ID
	}

	tagsMap := make(map[uuid.UUID][]TagResponse)
	if len(taskIDs) > 0 {
		tagRows, err := s.repo.GetTagsForTasks(ctx, taskIDs)
		if err == nil {
			for _, r := range tagRows {
				tagsMap[r.TaskID] = append(tagsMap[r.TaskID], TagResponse{
					ID:        r.TagID,
					Name:      r.Name,
					Color:     r.Color,
					CreatedAt: r.CreatedAt.Time,
				})
			}
		}
	}

	items := make([]TaskResponse, len(tasks))
	for i, t := range tasks {
		items[i] = *s.mapTask(t, nil)
		if assignedTags, exists := tagsMap[t.ID]; exists {
			items[i].Tags = assignedTags
		}
	}

	totalPages := int32(math.Ceil(float64(total) / float64(limit)))

	return &PaginatedTasksResponse{
		Items:      items,
		TotalItems: total,
		Page:       page,
		Limit:      limit,
		TotalPages: totalPages,
	}, nil
}

// UpdateTask updates task fields with optimistic lock version verification.
func (s *taskService) UpdateTask(ctx context.Context, userID uuid.UUID, taskID uuid.UUID, input UpdateTaskInput) (*TaskResponse, error) {
	existing, err := s.repo.GetTaskByID(ctx, taskID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrTaskNotFound
		}
		return nil, fmt.Errorf("failed to check existing task: %w", err)
	}

	if existing.CreatorID != userID && (!existing.AssigneeID.Valid || existing.AssigneeID.Bytes != userID) {
		return nil, ErrForbidden
	}

	if existing.Version != input.Version {
		return nil, ErrOptimisticLockConflict
	}

	title := strings.TrimSpace(input.Title)
	if title == "" {
		title = existing.Title
	}

	description := strings.TrimSpace(input.Description)
	if description == "" {
		description = existing.Description
	}

	status := strings.ToLower(strings.TrimSpace(input.Status))
	if status == "" {
		status = existing.Status
	} else if !validStatuses[status] {
		return nil, ErrInvalidTaskStatus
	}

	priority := strings.ToLower(strings.TrimSpace(input.Priority))
	if priority == "" {
		priority = existing.Priority
	} else if !validPriorities[priority] {
		return nil, ErrInvalidTaskPriority
	}

	pID := existing.ProjectID
	if input.ProjectID != nil {
		pID = pgtype.UUID{Bytes: *input.ProjectID, Valid: true}
	}

	aID := existing.AssigneeID
	if input.AssigneeID != nil {
		aID = pgtype.UUID{Bytes: *input.AssigneeID, Valid: true}
	}

	dDate := existing.DueDate
	if input.DueDate != nil {
		dDate = pgtype.Timestamptz{Time: *input.DueDate, Valid: true}
	}

	updated, err := s.repo.UpdateTask(ctx, repository.UpdateTaskParams{
		ID:          taskID,
		ProjectID:   pID,
		AssigneeID:  aID,
		Title:       title,
		Description: description,
		Status:      status,
		Priority:    priority,
		DueDate:     dDate,
		Version:     input.Version,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrOptimisticLockConflict
		}
		return nil, fmt.Errorf("failed to update task: %w", err)
	}

	// Update tags if provided
	if input.TagIDs != nil {
		_ = s.repo.ClearTaskTags(ctx, taskID)
		for _, tagID := range input.TagIDs {
			_ = s.repo.AddTagToTask(ctx, repository.AddTagToTaskParams{
				TaskID: taskID,
				TagID:  tagID,
			})
		}
	}

	tags, _ := s.repo.GetTagsForTask(ctx, taskID)
	return s.mapTask(updated, tags), nil
}

// DeleteTask soft-deletes the task.
func (s *taskService) DeleteTask(ctx context.Context, userID uuid.UUID, taskID uuid.UUID) error {
	existing, err := s.repo.GetTaskByID(ctx, taskID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrTaskNotFound
		}
		return fmt.Errorf("failed to find task before deletion: %w", err)
	}

	if existing.CreatorID != userID {
		return ErrForbidden
	}

	if err := s.repo.SoftDeleteTask(ctx, taskID); err != nil {
		return fmt.Errorf("failed to soft delete task: %w", err)
	}

	return nil
}

func (s *taskService) mapTask(t repository.Task, tags []repository.Tag) *TaskResponse {
	var projectID *uuid.UUID
	if t.ProjectID.Valid {
		id := uuid.UUID(t.ProjectID.Bytes)
		projectID = &id
	}

	var assigneeID *uuid.UUID
	if t.AssigneeID.Valid {
		id := uuid.UUID(t.AssigneeID.Bytes)
		assigneeID = &id
	}

	var dueDate *time.Time
	if t.DueDate.Valid {
		d := t.DueDate.Time
		dueDate = &d
	}

	mappedTags := make([]TagResponse, len(tags))
	for i, tg := range tags {
		mappedTags[i] = TagResponse{
			ID:        tg.ID,
			UserID:    tg.UserID,
			Name:      tg.Name,
			Color:     tg.Color,
			CreatedAt: tg.CreatedAt.Time,
		}
	}

	return &TaskResponse{
		ID:          t.ID,
		ProjectID:   projectID,
		CreatorID:   t.CreatorID,
		AssigneeID:  assigneeID,
		Title:       t.Title,
		Description: t.Description,
		Status:      t.Status,
		Priority:    t.Priority,
		DueDate:     dueDate,
		Version:     t.Version,
		Tags:        mappedTags,
		CreatedAt:   t.CreatedAt.Time,
		UpdatedAt:   t.UpdatedAt.Time,
	}
}
