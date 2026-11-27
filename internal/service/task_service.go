package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"task-manager-api/internal/authz"
	"task-manager-api/internal/domain"
	"task-manager-api/internal/repository"
	"task-manager-api/pkg/pagination"
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
	TagID     *uuid.UUID `json:"tag_id"`
	Search    *string    `json:"search"`
	DueBefore *time.Time `json:"due_before"`
	Cursor    string     `json:"cursor"`
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
	Page       int32          `json:"page,omitempty"`
	Limit      int32          `json:"limit"`
	TotalPages int32          `json:"total_pages,omitempty"`
	NextCursor string         `json:"next_cursor,omitempty"`
	HasMore    bool           `json:"has_more"`
}

// TaskService defines business logic for tasks.
type TaskService interface {
	CreateTask(ctx context.Context, userID uuid.UUID, input CreateTaskInput) (*TaskResponse, error)
	GetTaskByID(ctx context.Context, userID uuid.UUID, taskID uuid.UUID) (*TaskResponse, error)
	ListTasks(ctx context.Context, userID uuid.UUID, filter ListTasksFilter) (*PaginatedTasksResponse, error)
	ListProjectTasks(ctx context.Context, userID uuid.UUID, projectID uuid.UUID, filter ListTasksFilter) (*PaginatedTasksResponse, error)
	UpdateTask(ctx context.Context, userID uuid.UUID, taskID uuid.UUID, input UpdateTaskInput) (*TaskResponse, error)
	DeleteTask(ctx context.Context, userID uuid.UUID, taskID uuid.UUID) error
	SetTaskTags(ctx context.Context, userID uuid.UUID, taskID uuid.UUID, tagIDs []uuid.UUID) ([]TagResponse, error)
}

type taskService struct {
	repo       repository.Querier
	dispatcher EventDispatcher
}

// SetEventDispatcher registers an event dispatcher for webhook notifications.
func (s *taskService) SetEventDispatcher(dispatcher EventDispatcher) {
	s.dispatcher = dispatcher
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

	if input.DueDate != nil && input.DueDate.Before(time.Now().Add(-5*time.Minute)) {
		return nil, domain.ValidationErrors{
			Message: "Input validation failed",
			Params: []domain.InvalidParam{
				{Name: "due_date", Reason: "due date cannot be in the past"},
			},
		}
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
		if project.DeletedAt.Valid {
			return nil, ErrProjectNotFound
		}
		if project.OwnerID != userID && !s.checkProjectAccess(ctx, *input.ProjectID, userID, "owner", "editor") {
			return nil, ErrProjectNotFound // anti-IDOR: return 404 instead of 403
		}
		pID = pgtype.UUID{Bytes: *input.ProjectID, Valid: true}
	}

	var aID pgtype.UUID
	if input.AssigneeID != nil {
		if err := s.validateAssignee(ctx, input.ProjectID, input.AssigneeID); err != nil {
			return nil, err
		}
		aID = pgtype.UUID{Bytes: *input.AssigneeID, Valid: true}
	}

	var dDate pgtype.Timestamptz
	if input.DueDate != nil {
		dDate = pgtype.Timestamptz{Time: *input.DueDate, Valid: true}
	}

	var task repository.Task
	var tags []repository.Tag

	if store, ok := s.repo.(repository.Store); ok {
		err := store.WithTx(ctx, func(q repository.Querier) error {
			var err error
			task, err = q.CreateTask(ctx, repository.CreateTaskParams{
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
				return fmt.Errorf("failed to persist task: %w", err)
			}

			for _, tagID := range input.TagIDs {
				_ = q.AddTagToTask(ctx, repository.AddTagToTaskParams{
					TaskID: task.ID,
					TagID:  tagID,
				})
			}

			tags, _ = q.GetTagsForTask(ctx, task.ID)
			payloadBytes, err := json.Marshal(s.mapTask(task, tags))
			if err != nil {
				return fmt.Errorf("failed to marshal outbox event payload: %w", err)
			}

			_, err = q.CreateOutboxEvent(ctx, repository.CreateOutboxEventParams{
				EventType:     "task.created",
				AggregateType: "task",
				AggregateID:   task.ID,
				ActorID:       userID,
				ProjectID:     pID,
				Payload:       payloadBytes,
			})
			if err != nil {
				return fmt.Errorf("failed to insert outbox event: %w", err)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	} else {
		var err error
		task, err = s.repo.CreateTask(ctx, repository.CreateTaskParams{
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

		for _, tagID := range input.TagIDs {
			_ = s.repo.AddTagToTask(ctx, repository.AddTagToTaskParams{
				TaskID: task.ID,
				TagID:  tagID,
			})
		}
		tags, _ = s.repo.GetTagsForTask(ctx, task.ID)
		payloadBytes, _ := json.Marshal(s.mapTask(task, tags))
		_, _ = s.repo.CreateOutboxEvent(ctx, repository.CreateOutboxEventParams{
			EventType:     "task.created",
			AggregateType: "task",
			AggregateID:   task.ID,
			ActorID:       userID,
			ProjectID:     pID,
			Payload:       payloadBytes,
		})
	}

	resp := s.mapTask(task, tags)
	return resp, nil
}

// loadTaskResource retrieves task details and constructs the authorization Resource.
func (s *taskService) loadTaskResource(ctx context.Context, userID uuid.UUID, task repository.Task) (authz.Resource, error) {
	res := authz.Resource{
		TaskID:    task.ID,
		CreatorID: task.CreatorID,
	}
	if task.AssigneeID.Valid {
		assigneeID := uuid.UUID(task.AssigneeID.Bytes)
		res.AssigneeID = &assigneeID
	}

	if task.ProjectID.Valid {
		projectID := uuid.UUID(task.ProjectID.Bytes)
		res.ProjectID = &projectID

		project, pErr := s.repo.GetProjectByID(ctx, projectID)
		if pErr != nil {
			if errors.Is(pErr, pgx.ErrNoRows) {
				return authz.Resource{}, ErrTaskNotFound
			}
			return authz.Resource{}, fmt.Errorf("failed to get project: %w", pErr)
		}

		if project.DeletedAt.Valid {
			res.ProjectDeleted = true
		}

		if project.OwnerID == userID {
			res.MemberRole = authz.RolePtr(authz.RoleOwner)
		} else {
			member, mErr := s.repo.GetProjectMember(ctx, repository.GetProjectMemberParams{
				ProjectID: projectID,
				UserID:    userID,
			})
			if mErr == nil {
				res.MemberRole = authz.ParseRole(member.Role)
			}
		}
	}

	return res, nil
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

	if task.DeletedAt.Valid {
		return nil, ErrTaskNotFound
	}

	res, err := s.loadTaskResource(ctx, userID, task)
	if err != nil {
		return nil, err
	}

	sub := authz.Subject{UserID: userID}
	if !authz.Can(sub, res, authz.ActionRead) {
		return nil, ErrTaskNotFound // anti-IDOR: return 404 instead of 403
	}

	tags, err := s.repo.GetTagsForTask(ctx, task.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to get task tags: %w", err)
	}

	return s.mapTask(task, tags), nil
}

// ListProjectTasks lists all tasks belonging to a specific project after verifying access.
func (s *taskService) ListProjectTasks(ctx context.Context, userID uuid.UUID, projectID uuid.UUID, filter ListTasksFilter) (*PaginatedTasksResponse, error) {
	// Verify project access (anti-IDOR: return 404 if project doesn't exist, is soft-deleted, or user lacks access)
	project, err := s.repo.GetProjectByID(ctx, projectID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrProjectNotFound
		}
		return nil, fmt.Errorf("failed to fetch project: %w", err)
	}
	if project.DeletedAt.Valid {
		return nil, ErrProjectNotFound
	}
	if project.OwnerID != userID && !s.checkProjectAccess(ctx, projectID, userID, "owner", "editor", "viewer") {
		return nil, ErrProjectNotFound // anti-IDOR
	}

	filter.ProjectID = &projectID
	return s.ListTasks(ctx, userID, filter)
}

// ListTasks retrieves paginated tasks filtered by project, status, priority, search, tag, due_before.
func (s *taskService) ListTasks(ctx context.Context, userID uuid.UUID, filter ListTasksFilter) (*PaginatedTasksResponse, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = 20
	} else if limit > 100 {
		limit = 100
	}

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

	var search pgtype.Text
	if filter.Search != nil && strings.TrimSpace(*filter.Search) != "" {
		search = pgtype.Text{String: strings.TrimSpace(*filter.Search), Valid: true}
	}

	var dueBefore pgtype.Timestamptz
	if filter.DueBefore != nil {
		dueBefore = pgtype.Timestamptz{Time: *filter.DueBefore, Valid: true}
	}

	var tagID pgtype.UUID
	if filter.TagID != nil {
		tagID = pgtype.UUID{Bytes: *filter.TagID, Valid: true}
	}

	// 1. Keyset cursor-based pagination
	if filter.Cursor != "" {
		cursorTime, cursorID, err := pagination.DecodeCursor(filter.Cursor)
		if err != nil {
			return nil, domain.ValidationErrors{
				Message: "Input validation failed",
				Params: []domain.InvalidParam{
					{Name: "cursor", Reason: "invalid pagination cursor"},
				},
			}
		}

		keysetRows, err := s.repo.ListTasksKeyset(ctx, repository.ListTasksKeysetParams{
			CreatorID:       userID,
			Limit:           limit + 1,
			ProjectID:       pID,
			Status:          status,
			Priority:        priority,
			DueBefore:       dueBefore,
			Search:          search,
			TagID:           tagID,
			CursorCreatedAt: pgtype.Timestamptz{Time: cursorTime, Valid: true},
			CursorID:        pgtype.UUID{Bytes: cursorID, Valid: true},
		})
		if err != nil {
			return nil, fmt.Errorf("failed to query keyset tasks: %w", err)
		}

		hasMore := len(keysetRows) > int(limit)
		if hasMore {
			keysetRows = keysetRows[:limit]
		}

		var nextCursor string
		if len(keysetRows) > 0 && hasMore {
			last := keysetRows[len(keysetRows)-1]
			nextCursor = pagination.EncodeCursor(last.CreatedAt.Time, last.ID)
		}

		items := s.mapTasksWithTags(ctx, keysetRows)

		return &PaginatedTasksResponse{
			Items:      items,
			TotalItems: int64(len(items)),
			Limit:      limit,
			NextCursor: nextCursor,
			HasMore:    hasMore,
		}, nil
	}

	// 2. Traditional offset pagination
	page := filter.Page
	if page <= 0 {
		page = 1
	}
	offset := (page - 1) * limit

	total, err := s.repo.CountTasks(ctx, repository.CountTasksParams{
		CreatorID: userID,
		ProjectID: pID,
		Status:    status,
		Priority:  priority,
		DueBefore: dueBefore,
		Search:    search,
		TagID:     tagID,
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
		DueBefore: dueBefore,
		Search:    search,
		TagID:     tagID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list tasks: %w", err)
	}

	items := s.mapTasksWithTags(ctx, tasks)
	totalPages := int32(math.Ceil(float64(total) / float64(limit)))

	hasMore := int64(offset+limit) < total
	var nextCursor string
	if len(tasks) > 0 && hasMore {
		last := tasks[len(tasks)-1]
		nextCursor = pagination.EncodeCursor(last.CreatedAt.Time, last.ID)
	}

	return &PaginatedTasksResponse{
		Items:      items,
		TotalItems: total,
		Page:       page,
		Limit:      limit,
		TotalPages: totalPages,
		NextCursor: nextCursor,
		HasMore:    hasMore,
	}, nil
}

func (s *taskService) mapTasksWithTags(ctx context.Context, tasks []repository.Task) []TaskResponse {
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
	return items
}

// SetTaskTags replaces all tags for a task within a transaction.
func (s *taskService) SetTaskTags(ctx context.Context, userID uuid.UUID, taskID uuid.UUID, tagIDs []uuid.UUID) ([]TagResponse, error) {
	// First verify task exists and user has access
	task, err := s.repo.GetTaskByID(ctx, taskID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrTaskNotFound
		}
		return nil, fmt.Errorf("failed to get task: %w", err)
	}

	if task.DeletedAt.Valid {
		return nil, ErrTaskNotFound
	}

	res, err := s.loadTaskResource(ctx, userID, task)
	if err != nil {
		return nil, err
	}

	sub := authz.Subject{UserID: userID}
	if !authz.Can(sub, res, authz.ActionRead) {
		return nil, ErrTaskNotFound // anti-IDOR: return 404 instead of 403
	}

	if !authz.Can(sub, res, authz.ActionEdit) {
		return nil, ErrInsufficientProjectRole
	}

	// Use transaction if store supports WithTx
	if store, ok := s.repo.(repository.Store); ok {
		var updatedTags []TagResponse
		err := store.WithTx(ctx, func(q repository.Querier) error {
			if err := q.ClearTaskTags(ctx, taskID); err != nil {
				return fmt.Errorf("failed to clear tags: %w", err)
			}
			for _, tagID := range tagIDs {
				if err := q.AddTagToTask(ctx, repository.AddTagToTaskParams{
					TaskID: taskID,
					TagID:  tagID,
				}); err != nil {
					return fmt.Errorf("failed to add tag: %w", err)
				}
			}
			tags, err := q.GetTagsForTask(ctx, taskID)
			if err != nil {
				return fmt.Errorf("failed to fetch updated tags: %w", err)
			}
			for _, t := range tags {
				updatedTags = append(updatedTags, TagResponse{
					ID:        t.ID,
					Name:      t.Name,
					Color:     t.Color,
					CreatedAt: t.CreatedAt.Time,
				})
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
		return updatedTags, nil
	}

	// Fallback without transaction wrapper
	if err := s.repo.ClearTaskTags(ctx, taskID); err != nil {
		return nil, fmt.Errorf("failed to clear tags: %w", err)
	}
	for _, tagID := range tagIDs {
		_ = s.repo.AddTagToTask(ctx, repository.AddTagToTaskParams{
			TaskID: taskID,
			TagID:  tagID,
		})
	}
	tags, err := s.repo.GetTagsForTask(ctx, taskID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch updated tags: %w", err)
	}
	var updatedTagResponses []TagResponse
	for _, t := range tags {
		updatedTagResponses = append(updatedTagResponses, TagResponse{
			ID:        t.ID,
			Name:      t.Name,
			Color:     t.Color,
			CreatedAt: t.CreatedAt.Time,
		})
	}
	return updatedTagResponses, nil
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

	if existing.DeletedAt.Valid {
		return nil, ErrTaskNotFound
	}

	res, err := s.loadTaskResource(ctx, userID, existing)
	if err != nil {
		return nil, err
	}

	sub := authz.Subject{UserID: userID}
	if !authz.Can(sub, res, authz.ActionRead) {
		return nil, ErrTaskNotFound // anti-IDOR: return 404 instead of 403
	}

	if !authz.Can(sub, res, authz.ActionEdit) {
		return nil, ErrInsufficientProjectRole
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

	var targetProjectID *uuid.UUID
	if input.ProjectID != nil {
		targetProjectID = input.ProjectID
	} else if existing.ProjectID.Valid {
		existingProjUUID := uuid.UUID(existing.ProjectID.Bytes)
		targetProjectID = &existingProjUUID
	}

	aID := existing.AssigneeID
	if input.AssigneeID != nil {
		if err := s.validateAssignee(ctx, targetProjectID, input.AssigneeID); err != nil {
			return nil, err
		}
		aID = pgtype.UUID{Bytes: *input.AssigneeID, Valid: true}
	} else if input.ProjectID != nil && existing.AssigneeID.Valid {
		existingAssigneeUUID := uuid.UUID(existing.AssigneeID.Bytes)
		if err := s.validateAssignee(ctx, targetProjectID, &existingAssigneeUUID); err != nil {
			return nil, err
		}
	}

	dDate := existing.DueDate
	if input.DueDate != nil {
		dDate = pgtype.Timestamptz{Time: *input.DueDate, Valid: true}
	}

	var updated repository.Task
	var tags []repository.Tag
	if store, ok := s.repo.(repository.Store); ok {
		err := store.WithTx(ctx, func(q repository.Querier) error {
			var err error
			updated, err = q.UpdateTask(ctx, repository.UpdateTaskParams{
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
					return ErrOptimisticLockConflict
				}
				return fmt.Errorf("failed to update task: %w", err)
			}

			if input.TagIDs != nil {
				_ = q.ClearTaskTags(ctx, taskID)
				for _, tagID := range input.TagIDs {
					_ = q.AddTagToTask(ctx, repository.AddTagToTaskParams{
						TaskID: taskID,
						TagID:  tagID,
					})
				}
			}

			tags, _ = q.GetTagsForTask(ctx, taskID)
			payloadBytes, err := json.Marshal(s.mapTask(updated, tags))
			if err != nil {
				return fmt.Errorf("failed to marshal outbox event payload: %w", err)
			}

			_, err = q.CreateOutboxEvent(ctx, repository.CreateOutboxEventParams{
				EventType:     "task.updated",
				AggregateType: "task",
				AggregateID:   taskID,
				ActorID:       userID,
				ProjectID:     updated.ProjectID,
				Payload:       payloadBytes,
			})
			if err != nil {
				return fmt.Errorf("failed to insert outbox event: %w", err)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	} else {
		var err error
		updated, err = s.repo.UpdateTask(ctx, repository.UpdateTaskParams{
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
			_, _ = s.SetTaskTags(ctx, userID, taskID, input.TagIDs)
		}

		tags, _ = s.repo.GetTagsForTask(ctx, taskID)
		payloadBytes, _ := json.Marshal(s.mapTask(updated, tags))
		_, _ = s.repo.CreateOutboxEvent(ctx, repository.CreateOutboxEventParams{
			EventType:     "task.updated",
			AggregateType: "task",
			AggregateID:   taskID,
			ActorID:       userID,
			ProjectID:     updated.ProjectID,
			Payload:       payloadBytes,
		})
	}

	resp := s.mapTask(updated, tags)
	return resp, nil
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

	if existing.DeletedAt.Valid {
		return ErrTaskNotFound
	}

	res, err := s.loadTaskResource(ctx, userID, existing)
	if err != nil {
		return err
	}

	sub := authz.Subject{UserID: userID}
	if !authz.Can(sub, res, authz.ActionRead) {
		return ErrTaskNotFound // anti-IDOR: return 404 instead of 403
	}

	if !authz.Can(sub, res, authz.ActionDelete) {
		return ErrInsufficientProjectRole
	}

	if store, ok := s.repo.(repository.Store); ok {
		err := store.WithTx(ctx, func(q repository.Querier) error {
			if err := q.SoftDeleteTask(ctx, taskID); err != nil {
				return fmt.Errorf("failed to delete task: %w", err)
			}

			payloadBytes, err := json.Marshal(map[string]any{"task_id": taskID})
			if err != nil {
				return fmt.Errorf("failed to marshal outbox event payload: %w", err)
			}

			_, err = q.CreateOutboxEvent(ctx, repository.CreateOutboxEventParams{
				EventType:     "task.deleted",
				AggregateType: "task",
				AggregateID:   taskID,
				ActorID:       userID,
				ProjectID:     existing.ProjectID,
				Payload:       payloadBytes,
			})
			if err != nil {
				return fmt.Errorf("failed to insert outbox event: %w", err)
			}
			return nil
		})
		if err != nil {
			return err
		}
	} else {
		if err := s.repo.SoftDeleteTask(ctx, taskID); err != nil {
			return fmt.Errorf("failed to delete task: %w", err)
		}
		payloadBytes, _ := json.Marshal(map[string]any{"task_id": taskID})
		_, _ = s.repo.CreateOutboxEvent(ctx, repository.CreateOutboxEventParams{
			EventType:     "task.deleted",
			AggregateType: "task",
			AggregateID:   taskID,
			ActorID:       userID,
			ProjectID:     existing.ProjectID,
			Payload:       payloadBytes,
		})
	}

	return nil
}

func (s *taskService) validateAssignee(ctx context.Context, projectID *uuid.UUID, assigneeID *uuid.UUID) error {
	if assigneeID == nil {
		return nil
	}

	// Verify assignee user exists to prevent foreign key errors (HTTP 500)
	_, err := s.repo.GetUserByID(ctx, *assigneeID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrAssigneeNotFound
		}
		return fmt.Errorf("failed to verify assignee user: %w", err)
	}

	// If task is within a project, verify assignee is owner or active project member
	if projectID != nil {
		if !s.checkProjectAccess(ctx, *projectID, *assigneeID) {
			return ErrAssigneeNotInProject
		}
	}

	return nil
}

func (s *taskService) checkProjectAccess(ctx context.Context, projectID uuid.UUID, userID uuid.UUID, allowedRoles ...string) bool {
	member, err := s.repo.GetProjectMember(ctx, repository.GetProjectMemberParams{
		ProjectID: projectID,
		UserID:    userID,
	})
	if err != nil {
		project, pErr := s.repo.GetProjectByID(ctx, projectID)
		if pErr == nil && project.OwnerID == userID {
			return true
		}
		return false
	}
	if len(allowedRoles) == 0 {
		return true
	}
	for _, r := range allowedRoles {
		if member.Role == r {
			return true
		}
	}
	return false
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
