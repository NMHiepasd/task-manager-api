package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"task-manager-api/internal/authz"
	"task-manager-api/internal/repository"
	"task-manager-api/pkg/storage"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Allowed file extensions for attachments and their canonical Content-Types.
var allowedExtensions = map[string]string{
	".pdf":  "application/pdf",
	".png":  "image/png",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".webp": "image/webp",
	".gif":  "image/gif",
	".txt":  "text/plain",
	".csv":  "text/csv",
	".docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
	".xlsx": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
	".zip":  "application/zip",
}

// dangerousMIMEs contains types that could lead to XSS, script injection, or executable execution in browsers.
var dangerousMIMEs = []string{
	"text/html",
	"text/xml",
	"application/xml",
	"application/xhtml+xml",
	"application/javascript",
	"text/javascript",
	"application/x-javascript",
	"image/svg+xml",
	"application/x-sh",
	"application/x-msdownload",
	"application/x-dosexec",
}

// isMIMECompatible validates that the sniffed MIME type is compatible with the file extension.
func isMIMECompatible(ext string, detectedMIME string) bool {
	baseMIME := strings.ToLower(strings.TrimSpace(strings.Split(detectedMIME, ";")[0]))

	for _, dangerous := range dangerousMIMEs {
		if baseMIME == dangerous || strings.HasPrefix(baseMIME, dangerous) {
			return false
		}
	}

	switch ext {
	case ".pdf":
		return baseMIME == "application/pdf"
	case ".png":
		return baseMIME == "image/png"
	case ".jpg", ".jpeg":
		return baseMIME == "image/jpeg"
	case ".webp":
		return baseMIME == "image/webp"
	case ".gif":
		return baseMIME == "image/gif"
	case ".docx", ".xlsx", ".zip":
		return baseMIME == "application/zip" || baseMIME == "application/octet-stream"
	case ".txt", ".csv":
		return strings.HasPrefix(baseMIME, "text/") || baseMIME == "application/octet-stream"
	default:
		return false
	}
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
	dispatcher         EventDispatcher
}

// SetEventDispatcher registers an event dispatcher for webhook notifications.
func (s *attachmentService) SetEventDispatcher(dispatcher EventDispatcher) {
	s.dispatcher = dispatcher
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

// UploadAttachment validates the file against allowlist, sniffs MIME type, checks authorization, uploads to storage, and saves metadata.
func (s *attachmentService) UploadAttachment(ctx context.Context, userID, taskID uuid.UUID, fileName string, fileSize int64, _ string, reader io.Reader) (*AttachmentResponse, error) {
	if fileSize <= 0 {
		return nil, ErrEmptyFile
	}
	if fileSize > s.maxUploadSizeBytes {
		return nil, ErrFileTooLarge
	}

	cleanFileName := sanitizeFileName(fileName)
	ext := strings.ToLower(filepath.Ext(cleanFileName))
	canonicalMIME, isAllowed := allowedExtensions[ext]
	if !isAllowed {
		return nil, ErrInvalidFileType
	}

	// Sniff MIME type from first 512 bytes to verify magic numbers and prevent file spoofing
	buf := make([]byte, 512)
	n, err := io.ReadFull(reader, buf)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return nil, fmt.Errorf("failed to read attachment header for content type detection: %w", err)
	}
	sniffedBytes := buf[:n]
	detectedMIME := http.DetectContentType(sniffedBytes)

	if !isMIMECompatible(ext, detectedMIME) {
		return nil, ErrInvalidFileType
	}

	// Reconstruct stream with sniffed bytes prepended
	fullReader := io.MultiReader(bytes.NewReader(sniffedBytes), reader)

	// Use verified canonical MIME type instead of untrusted client Content-Type
	safeContentType := canonicalMIME

	// Verify task write permission
	task, _, err := s.verifyTaskAccess(ctx, userID, taskID, authz.ActionUpload)
	if err != nil {
		return nil, err
	}

	// Generate safe unique storage key: tasks/{taskID}/{uuid}_{cleanFileName}
	attachmentID := uuid.New()
	storageKey := fmt.Sprintf("tasks/%s/%s_%s", taskID.String(), attachmentID.String(), cleanFileName)

	// Stream file to object storage
	if err := s.storage.Upload(ctx, storageKey, fullReader, fileSize, safeContentType); err != nil {
		return nil, fmt.Errorf("failed to upload attachment to storage: %w", err)
	}

	// Persist attachment metadata in DB
	record, err := s.queries.CreateTaskAttachment(ctx, repository.CreateTaskAttachmentParams{
		TaskID:      taskID,
		UserID:      userID,
		FileName:    cleanFileName,
		FileSize:    fileSize,
		ContentType: safeContentType,
		StorageKey:  storageKey,
	})
	if err != nil {
		// Rollback uploaded file from storage on database failure
		_ = s.storage.Delete(ctx, storageKey)
		return nil, fmt.Errorf("failed to save attachment record: %w", err)
	}

	resp := toAttachmentResponse(record)
	if s.dispatcher != nil {
		var pID *uuid.UUID
		if task.ProjectID.Valid {
			id := uuid.UUID(task.ProjectID.Bytes)
			pID = &id
		}
		_ = s.dispatcher.DispatchEvent(ctx, userID, pID, "attachment.uploaded", resp)
	}

	return resp, nil
}

// ListAttachments returns all attachments belonging to a task for authorized members.
func (s *attachmentService) ListAttachments(ctx context.Context, userID, taskID uuid.UUID) ([]AttachmentResponse, error) {
	if _, _, err := s.verifyTaskAccess(ctx, userID, taskID, authz.ActionRead); err != nil {
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
	if _, _, err := s.verifyTaskAccess(ctx, userID, taskID, authz.ActionRead); err != nil {
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
	if _, _, err := s.verifyTaskAccess(ctx, userID, taskID, authz.ActionRead); err != nil {
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
	_, res, err := s.verifyTaskAccess(ctx, userID, taskID, authz.ActionRead)
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
	// Allowed if caller is the attachment author (record.UserID == userID) or holds ActionModerateAttachments
	sub := authz.Subject{UserID: userID}
	canDelete := record.UserID == userID || authz.Can(sub, res, authz.ActionModerateAttachments)
	if !canDelete {
		return ErrInsufficientProjectRole
	}

	if err := s.queries.DeleteTaskAttachment(ctx, attachmentID); err != nil {
		return fmt.Errorf("failed to delete attachment from db: %w", err)
	}

	// Delete from storage (best-effort, log failure if any)
	_ = s.storage.Delete(ctx, record.StorageKey)

	return nil
}

// loadTaskResource retrieves task details and constructs the authorization Resource.
func (s *attachmentService) loadTaskResource(ctx context.Context, userID, taskID uuid.UUID) (*repository.Task, authz.Resource, error) {
	task, err := s.queries.GetTaskByID(ctx, taskID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, authz.Resource{}, ErrTaskNotFound
		}
		return nil, authz.Resource{}, fmt.Errorf("failed to query task: %w", err)
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

		project, pErr := s.queries.GetProjectByID(ctx, projectID)
		if pErr != nil {
			if errors.Is(pErr, pgx.ErrNoRows) {
				return nil, authz.Resource{}, ErrTaskNotFound
			}
			return nil, authz.Resource{}, fmt.Errorf("failed to query project: %w", pErr)
		}

		if project.DeletedAt.Valid {
			res.ProjectDeleted = true
		}

		if project.OwnerID == userID {
			res.MemberRole = authz.RolePtr(authz.RoleOwner)
		} else {
			member, mErr := s.queries.GetProjectMember(ctx, repository.GetProjectMemberParams{
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
func (s *attachmentService) verifyTaskAccess(ctx context.Context, userID, taskID uuid.UUID, action authz.Action) (*repository.Task, authz.Resource, error) {
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
