package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"task-manager-api/internal/repository"
	"task-manager-api/pkg/storage"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Blocked file extensions for safety against remote code execution / script injection.
var blockedExtensions = map[string]bool{
	".exe": true, ".dll": true, ".bat": true, ".sh": true,
	".cmd": true, ".com": true, ".msi": true, ".vbs": true,
	".scr": true, ".pif": true, ".app": true, ".jar": true,
	".iso": true,
}

// AttachmentResponse represents the public metadata DTO for a task file attachment.
type AttachmentResponse struct {
	ID          uuid.UUID `json:"id"`
	TaskID      uuid.UUID `json:"task_id"`
	UserID      uuid.UUID `json:"user_id"`
	FileName    string    `json:"file_name"`
	FileSize    int64     `json:"file_size"`
	ContentType string    `json:"content_type"`
	CreatedAt   time.Time `json:"created_at"`
}

// AttachmentService defines operations for managing task file attachments.
type AttachmentService interface {
	UploadAttachment(ctx context.Context, userID, taskID uuid.UUID, fileName string, fileSize int64, contentType string, reader io.Reader) (*AttachmentResponse, error)
	ListAttachments(ctx context.Context, userID, taskID uuid.UUID) ([]AttachmentResponse, error)
	GetAttachment(ctx context.Context, userID, taskID, attachmentID uuid.UUID) (*AttachmentResponse, io.ReadCloser, error)
	GetAttachmentPresignedURL(ctx context.Context, userID, taskID, attachmentID uuid.UUID, expiry time.Duration) (string, error)
	DeleteAttachment(ctx context.Context, userID, taskID, attachmentID uuid.UUID) error
}

type attachmentService struct {
	queries            repository.Querier
	storage            storage.Client
	maxUploadSizeBytes int64
}

// NewAttachmentService creates a new AttachmentService instance.
func NewAttachmentService(queries repository.Querier, storage storage.Client, maxUploadSizeMB int64) AttachmentService {
	if maxUploadSizeMB <= 0 {
		maxUploadSizeMB = 10
	}
	return &attachmentService{
		queries:            queries,
		storage:            storage,
		maxUploadSizeBytes: maxUploadSizeMB * 1024 * 1024,
	}
}

// UploadAttachment validates the file, checks authorization, uploads to storage, and saves metadata.
func (s *attachmentService) UploadAttachment(ctx context.Context, userID, taskID uuid.UUID, fileName string, fileSize int64, contentType string, reader io.Reader) (*AttachmentResponse, error) {
	if fileSize <= 0 {
		return nil, ErrEmptyFile
	}
	if fileSize > s.maxUploadSizeBytes {
		return nil, ErrFileTooLarge
	}

	cleanFileName := sanitizeFileName(fileName)
	ext := strings.ToLower(filepath.Ext(cleanFileName))
	if blockedExtensions[ext] {
		return nil, ErrInvalidFileType
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	// Verify task write permission
	_, err := s.verifyTaskWriteAccess(ctx, userID, taskID)
	if err != nil {
		return nil, err
	}

	// Generate safe unique storage key: tasks/{taskID}/{uuid}_{cleanFileName}
	attachmentID := uuid.New()
	storageKey := fmt.Sprintf("tasks/%s/%s_%s", taskID.String(), attachmentID.String(), cleanFileName)

	// Stream file to object storage
	if err := s.storage.Upload(ctx, storageKey, reader, fileSize, contentType); err != nil {
		return nil, fmt.Errorf("failed to upload attachment to storage: %w", err)
	}

	// Persist attachment metadata in DB
	record, err := s.queries.CreateTaskAttachment(ctx, repository.CreateTaskAttachmentParams{
		TaskID:      taskID,
		UserID:      userID,
		FileName:    cleanFileName,
		FileSize:    fileSize,
		ContentType: contentType,
		StorageKey:  storageKey,
	})
	if err != nil {
		// Rollback uploaded file from storage on database failure
		_ = s.storage.Delete(ctx, storageKey)
		return nil, fmt.Errorf("failed to save attachment record: %w", err)
	}

	return toAttachmentResponse(record), nil
}

// ListAttachments returns all attachments belonging to a task for authorized members.
func (s *attachmentService) ListAttachments(ctx context.Context, userID, taskID uuid.UUID) ([]AttachmentResponse, error) {
	if _, _, err := s.verifyTaskReadAccess(ctx, userID, taskID); err != nil {
		return nil, err
	}

	records, err := s.queries.ListTaskAttachmentsByTaskID(ctx, taskID)
	if err != nil {
		return nil, fmt.Errorf("failed to list attachments: %w", err)
	}

	responses := make([]AttachmentResponse, 0, len(records))
	for _, r := range records {
		responses = append(responses, *toAttachmentResponse(r))
	}

	return responses, nil
}

// GetAttachment retrieves attachment metadata and downloads stream for authorized viewers.
func (s *attachmentService) GetAttachment(ctx context.Context, userID, taskID, attachmentID uuid.UUID) (*AttachmentResponse, io.ReadCloser, error) {
	if _, _, err := s.verifyTaskReadAccess(ctx, userID, taskID); err != nil {
		return nil, nil, err
	}

	record, err := s.queries.GetTaskAttachmentByID(ctx, attachmentID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil, ErrAttachmentNotFound
		}
		return nil, nil, fmt.Errorf("failed to query attachment: %w", err)
	}

	if record.TaskID != taskID {
		return nil, nil, ErrAttachmentNotFound
	}

	stream, err := s.storage.Download(ctx, record.StorageKey)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to stream attachment from storage: %w", err)
	}

	return toAttachmentResponse(record), stream, nil
}

// GetAttachmentPresignedURL generates a temporary download URL for an attachment.
func (s *attachmentService) GetAttachmentPresignedURL(ctx context.Context, userID, taskID, attachmentID uuid.UUID, expiry time.Duration) (string, error) {
	if _, _, err := s.verifyTaskReadAccess(ctx, userID, taskID); err != nil {
		return "", err
	}

	record, err := s.queries.GetTaskAttachmentByID(ctx, attachmentID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrAttachmentNotFound
		}
		return "", fmt.Errorf("failed to query attachment: %w", err)
	}

	if record.TaskID != taskID {
		return "", ErrAttachmentNotFound
	}

	if expiry <= 0 {
		expiry = 15 * time.Minute
	}

	presignedURL, err := s.storage.GetPresignedURL(ctx, record.StorageKey, expiry)
	if err != nil {
		return "", fmt.Errorf("failed to generate presigned URL: %w", err)
	}

	return presignedURL, nil
}

// DeleteAttachment removes the attachment from DB and object storage following moderation rules.
func (s *attachmentService) DeleteAttachment(ctx context.Context, userID, taskID, attachmentID uuid.UUID) error {
	task, project, err := s.verifyTaskReadAccess(ctx, userID, taskID)
	if err != nil {
		return err
	}

	record, err := s.queries.GetTaskAttachmentByID(ctx, attachmentID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrAttachmentNotFound
		}
		return fmt.Errorf("failed to query attachment: %w", err)
	}

	if record.TaskID != taskID {
		return ErrAttachmentNotFound
	}

	// Moderation check:
	// Allowed if:
	// 1. Caller is the attachment author (record.UserID == userID)
	// 2. Caller is the task creator (task.CreatorID == userID)
	// 3. Caller is the project owner (project != nil && project.OwnerID == userID)
	isAuthor := record.UserID == userID
	isTaskCreator := task.CreatorID == userID
	isProjectOwner := project != nil && project.OwnerID == userID

	if !isAuthor && !isTaskCreator && !isProjectOwner {
		return ErrForbidden
	}

	if err := s.queries.DeleteTaskAttachment(ctx, attachmentID); err != nil {
		return fmt.Errorf("failed to delete attachment from db: %w", err)
	}

	// Delete from storage (best-effort, log failure if any)
	_ = s.storage.Delete(ctx, record.StorageKey)

	return nil
}

// verifyTaskReadAccess checks if the user has read access to the task.
func (s *attachmentService) verifyTaskReadAccess(ctx context.Context, userID, taskID uuid.UUID) (*repository.Task, *repository.Project, error) {
	task, err := s.queries.GetTaskByID(ctx, taskID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil, ErrTaskNotFound
		}
		return nil, nil, fmt.Errorf("failed to query task: %w", err)
	}

	if task.DeletedAt.Valid {
		return nil, nil, ErrTaskNotFound
	}

	var project *repository.Project
	if task.ProjectID.Valid {
		p, err := s.queries.GetProjectByID(ctx, task.ProjectID.Bytes)
		if err != nil {
			if !errors.Is(err, pgx.ErrNoRows) {
				return nil, nil, fmt.Errorf("failed to query project: %w", err)
			}
		} else if !p.DeletedAt.Valid {
			project = &p
			if p.OwnerID == userID {
				return &task, project, nil
			}
			member, err := s.queries.GetProjectMember(ctx, repository.GetProjectMemberParams{
				ProjectID: p.ID,
				UserID:    userID,
			})
			if err == nil && member.Role != "" {
				return &task, project, nil
			}
		}
	}

	if task.CreatorID == userID || (task.AssigneeID.Valid && task.AssigneeID.Bytes == userID) {
		return &task, project, nil
	}

	// Anti-IDOR: Return ErrTaskNotFound if user has no relationship with the task
	return nil, nil, ErrTaskNotFound
}

// verifyTaskWriteAccess checks if the user has write access to upload attachments.
func (s *attachmentService) verifyTaskWriteAccess(ctx context.Context, userID, taskID uuid.UUID) (*repository.Task, error) {
	task, project, err := s.verifyTaskReadAccess(ctx, userID, taskID)
	if err != nil {
		return nil, err
	}

	// If project-level task, verify member has Editor or Owner role (Viewers cannot upload attachments)
	if project != nil {
		if project.OwnerID == userID {
			return task, nil
		}
		member, err := s.queries.GetProjectMember(ctx, repository.GetProjectMemberParams{
			ProjectID: project.ID,
			UserID:    userID,
		})
		if err == nil {
			if member.Role == "viewer" {
				return nil, ErrInsufficientProjectRole
			}
			if member.Role == "editor" || member.Role == "owner" {
				return task, nil
			}
		}
	}

	// If personal task or direct creator/assignee without restrictive project role
	if task.CreatorID == userID || (task.AssigneeID.Valid && task.AssigneeID.Bytes == userID) {
		return task, nil
	}

	return nil, ErrInsufficientProjectRole
}

func sanitizeFileName(name string) string {
	base := filepath.Base(name)
	base = strings.ReplaceAll(base, " ", "_")
	base = strings.ReplaceAll(base, "..", "")
	base = strings.Trim(base, "._-")
	if base == "" {
		return "attachment"
	}
	return base
}

func toAttachmentResponse(record repository.TaskAttachment) *AttachmentResponse {
	return &AttachmentResponse{
		ID:          record.ID,
		TaskID:      record.TaskID,
		UserID:      record.UserID,
		FileName:    record.FileName,
		FileSize:    record.FileSize,
		ContentType: record.ContentType,
		CreatedAt:   record.CreatedAt.Time,
	}
}
