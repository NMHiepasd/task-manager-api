package service

import (
	"context"
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

// CreateCommentInput holds payload for creating a task comment.
type CreateCommentInput struct {
	Content string `json:"content"`
}

// UpdateCommentInput holds payload for updating a task comment.
type UpdateCommentInput struct {
	Content string `json:"content"`
}

// CommentResponse represents public comment data.
type CommentResponse struct {
	ID        uuid.UUID `json:"id"`
	TaskID    uuid.UUID `json:"task_id"`
	UserID    uuid.UUID `json:"user_id"`
	Author    string    `json:"author"`
	Email     string    `json:"email"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// PaginatedCommentsResponse represents paginated comment results.
type PaginatedCommentsResponse struct {
	Items      []CommentResponse `json:"items"`
	TotalItems int64             `json:"total_items"`
	Page       int32             `json:"page,omitempty"`
	Limit      int32             `json:"limit"`
	TotalPages int32             `json:"total_pages,omitempty"`
	NextCursor string            `json:"next_cursor,omitempty"`
	HasMore    bool              `json:"has_more"`
}

// CommentService defines business logic for managing task comments.
type CommentService interface {
	CreateComment(ctx context.Context, userID uuid.UUID, taskID uuid.UUID, input CreateCommentInput) (*CommentResponse, error)
	ListComments(ctx context.Context, userID uuid.UUID, taskID uuid.UUID, params PaginationParams) (*PaginatedCommentsResponse, error)
	UpdateComment(ctx context.Context, userID uuid.UUID, taskID uuid.UUID, commentID uuid.UUID, input UpdateCommentInput) (*CommentResponse, error)
	DeleteComment(ctx context.Context, userID uuid.UUID, taskID uuid.UUID, commentID uuid.UUID) error
}

type commentService struct {
	repo       repository.Querier
	dispatcher EventDispatcher
}

// SetEventDispatcher registers an event dispatcher for webhook notifications.
func (s *commentService) SetEventDispatcher(dispatcher EventDispatcher) {
	s.dispatcher = dispatcher
}

// NewCommentService constructs a new CommentService.
func NewCommentService(repo repository.Querier) CommentService {
	return &commentService{repo: repo}
}

// loadTaskResource retrieves task details and constructs the authorization Resource.
func (s *commentService) loadTaskResource(ctx context.Context, userID uuid.UUID, taskID uuid.UUID) (*repository.Task, authz.Resource, error) {
	task, err := s.repo.GetTaskByID(ctx, taskID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, authz.Resource{}, ErrTaskNotFound
		}
		return nil, authz.Resource{}, fmt.Errorf("failed to get task: %w", err)
	}

	if task.DeletedAt.Valid {
		return nil, authz.Resource{}, ErrTaskNotFound
	}

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
				return nil, authz.Resource{}, ErrTaskNotFound
			}
			return nil, authz.Resource{}, fmt.Errorf("failed to get project: %w", pErr)
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

	return &task, res, nil
}

// verifyTaskAccess checks whether the user has access to the task according to authz policy.
func (s *commentService) verifyTaskAccess(ctx context.Context, userID uuid.UUID, taskID uuid.UUID, action authz.Action) (*repository.Task, authz.Resource, error) {
	task, res, err := s.loadTaskResource(ctx, userID, taskID)
	if err != nil {
		return nil, authz.Resource{}, err
	}

	sub := authz.Subject{UserID: userID}
	if !authz.Can(sub, res, authz.ActionRead) {
		return nil, authz.Resource{}, ErrTaskNotFound // anti-IDOR
	}

	if !authz.Can(sub, res, action) {
		return nil, authz.Resource{}, ErrInsufficientProjectRole
	}

	return task, res, nil
}

// CreateComment adds a comment to a task.
func (s *commentService) CreateComment(ctx context.Context, userID uuid.UUID, taskID uuid.UUID, input CreateCommentInput) (*CommentResponse, error) {
	content := strings.TrimSpace(input.Content)
	if content == "" {
		return nil, ErrEmptyCommentContent
	}

	task, _, err := s.verifyTaskAccess(ctx, userID, taskID, authz.ActionComment)
	if err != nil {
		return nil, err
	}

	created, err := s.repo.CreateTaskComment(ctx, repository.CreateTaskCommentParams{
		TaskID:  taskID,
		UserID:  userID,
		Content: content,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create comment: %w", err)
	}

	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user details: %w", err)
	}

	resp := &CommentResponse{
		ID:        created.ID,
		TaskID:    created.TaskID,
		UserID:    created.UserID,
		Author:    user.FullName,
		Email:     user.Email,
		Content:   created.Content,
		CreatedAt: created.CreatedAt.Time,
		UpdatedAt: created.UpdatedAt.Time,
	}

	if s.dispatcher != nil {
		var pID *uuid.UUID
		if task.ProjectID.Valid {
			id := uuid.UUID(task.ProjectID.Bytes)
			pID = &id
		}
		_ = s.dispatcher.DispatchEvent(ctx, userID, pID, "comment.created", resp)
	}

	return resp, nil
}

// ListComments retrieves paginated comments for a task.
func (s *commentService) ListComments(ctx context.Context, userID uuid.UUID, taskID uuid.UUID, params PaginationParams) (*PaginatedCommentsResponse, error) {
	if _, _, err := s.verifyTaskAccess(ctx, userID, taskID, authz.ActionRead); err != nil {
		return nil, err
	}

	limit := params.Limit
	if limit <= 0 {
		limit = 10
	} else if limit > 100 {
		limit = 100
	}

	if params.Cursor != "" {
		cursorTime, cursorID, err := pagination.DecodeCursor(params.Cursor)
		if err != nil {
			return nil, domain.ValidationErrors{
				Message: "Input validation failed",
				Params: []domain.InvalidParam{
					{Name: "cursor", Reason: "invalid pagination cursor"},
				},
			}
		}

		rows, err := s.repo.ListTaskCommentsKeyset(ctx, repository.ListTaskCommentsKeysetParams{
			TaskID:          taskID,
			Limit:           limit + 1,
			CursorCreatedAt: pgtype.Timestamptz{Time: cursorTime, Valid: true},
			CursorID:        pgtype.UUID{Bytes: cursorID, Valid: true},
		})
		if err != nil {
			return nil, fmt.Errorf("failed to query keyset comments: %w", err)
		}

		hasMore := len(rows) > int(limit)
		if hasMore {
			rows = rows[:limit]
		}

		var nextCursor string
		if len(rows) > 0 && hasMore {
			last := rows[len(rows)-1]
			nextCursor = pagination.EncodeCursor(last.CreatedAt.Time, last.ID)
		}

		items := make([]CommentResponse, len(rows))
		for i, r := range rows {
			items[i] = CommentResponse{
				ID:        r.ID,
				TaskID:    r.TaskID,
				UserID:    r.UserID,
				Author:    r.FullName,
				Email:     r.Email,
				Content:   r.Content,
				CreatedAt: r.CreatedAt.Time,
				UpdatedAt: r.UpdatedAt.Time,
			}
		}

		return &PaginatedCommentsResponse{
			Items:      items,
			TotalItems: int64(len(items)),
			Limit:      limit,
			NextCursor: nextCursor,
			HasMore:    hasMore,
		}, nil
	}

	page := params.Page
	if page <= 0 {
		page = 1
	}
	offset := (page - 1) * limit

	total, err := s.repo.CountTaskComments(ctx, taskID)
	if err != nil {
		return nil, fmt.Errorf("failed to count comments: %w", err)
	}

	rows, err := s.repo.ListTaskComments(ctx, repository.ListTaskCommentsParams{
		TaskID: taskID,
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list comments: %w", err)
	}

	items := make([]CommentResponse, len(rows))
	for i, r := range rows {
		items[i] = CommentResponse{
			ID:        r.ID,
			TaskID:    r.TaskID,
			UserID:    r.UserID,
			Author:    r.FullName,
			Email:     r.Email,
			Content:   r.Content,
			CreatedAt: r.CreatedAt.Time,
			UpdatedAt: r.UpdatedAt.Time,
		}
	}

	totalPages := int32(math.Ceil(float64(total) / float64(limit)))
	hasMore := int64(offset+limit) < total
	var nextCursor string
	if len(rows) > 0 && hasMore {
		last := rows[len(rows)-1]
		nextCursor = pagination.EncodeCursor(last.CreatedAt.Time, last.ID)
	}

	return &PaginatedCommentsResponse{
		Items:      items,
		TotalItems: total,
		Page:       page,
		Limit:      limit,
		TotalPages: totalPages,
		NextCursor: nextCursor,
		HasMore:    hasMore,
	}, nil
}

// UpdateComment modifies comment content. Only the original author can edit a comment.
func (s *commentService) UpdateComment(ctx context.Context, userID uuid.UUID, taskID uuid.UUID, commentID uuid.UUID, input UpdateCommentInput) (*CommentResponse, error) {
	content := strings.TrimSpace(input.Content)
	if content == "" {
		return nil, ErrEmptyCommentContent
	}

	if _, _, err := s.verifyTaskAccess(ctx, userID, taskID, authz.ActionRead); err != nil {
		return nil, err
	}

	existing, err := s.repo.GetTaskCommentByID(ctx, commentID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrCommentNotFound
		}
		return nil, fmt.Errorf("failed to get comment: %w", err)
	}

	if existing.TaskID != taskID {
		return nil, ErrCommentNotFound // anti-IDOR
	}

	if existing.UserID != userID {
		return nil, ErrInsufficientProjectRole
	}

	updated, err := s.repo.UpdateTaskComment(ctx, repository.UpdateTaskCommentParams{
		ID:      commentID,
		Content: content,
		UserID:  userID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to update comment: %w", err)
	}

	return &CommentResponse{
		ID:        updated.ID,
		TaskID:    updated.TaskID,
		UserID:    updated.UserID,
		Author:    existing.FullName,
		Email:     existing.Email,
		Content:   updated.Content,
		CreatedAt: updated.CreatedAt.Time,
		UpdatedAt: updated.UpdatedAt.Time,
	}, nil
}

// DeleteComment removes a comment. The original author, task creator (personal task), or project owner can delete it.
func (s *commentService) DeleteComment(ctx context.Context, userID uuid.UUID, taskID uuid.UUID, commentID uuid.UUID) error {
	_, res, err := s.verifyTaskAccess(ctx, userID, taskID, authz.ActionRead)
	if err != nil {
		return err
	}

	existing, err := s.repo.GetTaskCommentByID(ctx, commentID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrCommentNotFound
		}
		return fmt.Errorf("failed to get comment: %w", err)
	}

	if existing.TaskID != taskID {
		return ErrCommentNotFound // anti-IDOR
	}

	sub := authz.Subject{UserID: userID}
	canDelete := existing.UserID == userID || authz.Can(sub, res, authz.ActionModerateComments)
	if !canDelete {
		return ErrInsufficientProjectRole
	}

	if err := s.repo.DeleteTaskComment(ctx, commentID); err != nil {
		return fmt.Errorf("failed to delete comment: %w", err)
	}

	return nil
}
