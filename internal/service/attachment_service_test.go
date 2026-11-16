package service

import (
	"bytes"
	"context"
	"io"
	"strings"
	"task-manager-api/internal/repository"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockStorageClient struct {
	uploadFunc          func(ctx context.Context, key string, reader io.Reader, size int64, contentType string) error
	downloadFunc        func(ctx context.Context, key string) (io.ReadCloser, error)
	deleteFunc          func(ctx context.Context, key string) error
	getPresignedURLFunc func(ctx context.Context, key string, expiry time.Duration) (string, error)
	ensureBucketFunc    func(ctx context.Context) error
}

func (m *mockStorageClient) Upload(ctx context.Context, key string, reader io.Reader, size int64, contentType string) error {
	if m.uploadFunc != nil {
		return m.uploadFunc(ctx, key, reader, size, contentType)
	}
	return nil
}

func (m *mockStorageClient) Download(ctx context.Context, key string) (io.ReadCloser, error) {
	if m.downloadFunc != nil {
		return m.downloadFunc(ctx, key)
	}
	return io.NopCloser(bytes.NewReader([]byte("mock content"))), nil
}

func (m *mockStorageClient) Delete(ctx context.Context, key string) error {
	if m.deleteFunc != nil {
		return m.deleteFunc(ctx, key)
	}
	return nil
}

func (m *mockStorageClient) GetPresignedURL(ctx context.Context, key string, expiry time.Duration) (string, error) {
	if m.getPresignedURLFunc != nil {
		return m.getPresignedURLFunc(ctx, key, expiry)
	}
	return "https://storage.local/presigned-url", nil
}

func (m *mockStorageClient) EnsureBucket(ctx context.Context) error {
	if m.ensureBucketFunc != nil {
		return m.ensureBucketFunc(ctx)
	}
	return nil
}

type mockAttachmentQuerier struct {
	mockQuerier
	getTaskByIDFn                 func(ctx context.Context, id uuid.UUID) (repository.Task, error)
	getProjectByIDFn              func(ctx context.Context, id uuid.UUID) (repository.Project, error)
	getProjectMemberFn            func(ctx context.Context, arg repository.GetProjectMemberParams) (repository.ProjectMember, error)
	createTaskAttachmentFn        func(ctx context.Context, arg repository.CreateTaskAttachmentParams) (repository.TaskAttachment, error)
	getTaskAttachmentByIDFn       func(ctx context.Context, id uuid.UUID) (repository.TaskAttachment, error)
	listTaskAttachmentsByTaskIDFn func(ctx context.Context, id uuid.UUID) ([]repository.TaskAttachment, error)
	deleteTaskAttachmentFn        func(ctx context.Context, id uuid.UUID) error
}

func (m *mockAttachmentQuerier) GetTaskByID(ctx context.Context, id uuid.UUID) (repository.Task, error) {
	if m.getTaskByIDFn != nil {
		return m.getTaskByIDFn(ctx, id)
	}
	return repository.Task{}, pgx.ErrNoRows
}

func (m *mockAttachmentQuerier) GetProjectByID(ctx context.Context, id uuid.UUID) (repository.Project, error) {
	if m.getProjectByIDFn != nil {
		return m.getProjectByIDFn(ctx, id)
	}
	return repository.Project{}, pgx.ErrNoRows
}

func (m *mockAttachmentQuerier) GetProjectMember(ctx context.Context, arg repository.GetProjectMemberParams) (repository.ProjectMember, error) {
	if m.getProjectMemberFn != nil {
		return m.getProjectMemberFn(ctx, arg)
	}
	return repository.ProjectMember{}, pgx.ErrNoRows
}

func (m *mockAttachmentQuerier) CreateTaskAttachment(ctx context.Context, arg repository.CreateTaskAttachmentParams) (repository.TaskAttachment, error) {
	if m.createTaskAttachmentFn != nil {
		return m.createTaskAttachmentFn(ctx, arg)
	}
	return repository.TaskAttachment{}, nil
}

func (m *mockAttachmentQuerier) GetTaskAttachmentByID(ctx context.Context, id uuid.UUID) (repository.TaskAttachment, error) {
	if m.getTaskAttachmentByIDFn != nil {
		return m.getTaskAttachmentByIDFn(ctx, id)
	}
	return repository.TaskAttachment{}, pgx.ErrNoRows
}

func (m *mockAttachmentQuerier) ListTaskAttachmentsByTaskID(ctx context.Context, id uuid.UUID) ([]repository.TaskAttachment, error) {
	if m.listTaskAttachmentsByTaskIDFn != nil {
		return m.listTaskAttachmentsByTaskIDFn(ctx, id)
	}
	return nil, nil
}

func (m *mockAttachmentQuerier) DeleteTaskAttachment(ctx context.Context, id uuid.UUID) error {
	if m.deleteTaskAttachmentFn != nil {
		return m.deleteTaskAttachmentFn(ctx, id)
	}
	return nil
}

func TestAttachmentService_UploadAttachment(t *testing.T) {
	ctx := context.Background()
	taskID := uuid.New()
	creatorID := uuid.New()
	viewerID := uuid.New()
	projectID := uuid.New()

	task := repository.Task{
		ID:        taskID,
		CreatorID: creatorID,
		ProjectID: pgtype.UUID{Bytes: projectID, Valid: true},
	}
	project := repository.Project{
		ID:      projectID,
		OwnerID: creatorID,
	}

	t.Run("Success", func(t *testing.T) {
		mockRepo := &mockAttachmentQuerier{
			getTaskByIDFn: func(_ context.Context, id uuid.UUID) (repository.Task, error) {
				if id == taskID {
					return task, nil
				}
				return repository.Task{}, pgx.ErrNoRows
			},
			getProjectByIDFn: func(_ context.Context, id uuid.UUID) (repository.Project, error) {
				if id == projectID {
					return project, nil
				}
				return repository.Project{}, pgx.ErrNoRows
			},
			createTaskAttachmentFn: func(_ context.Context, arg repository.CreateTaskAttachmentParams) (repository.TaskAttachment, error) {
				return repository.TaskAttachment{
					ID:          uuid.New(),
					TaskID:      arg.TaskID,
					UserID:      arg.UserID,
					FileName:    arg.FileName,
					FileSize:    arg.FileSize,
					ContentType: arg.ContentType,
					StorageKey:  arg.StorageKey,
					CreatedAt:   pgtype.Timestamptz{Time: time.Now(), Valid: true},
				}, nil
			},
		}
		mockStore := &mockStorageClient{}
		svc := NewAttachmentService(mockRepo, mockStore, 10)

		resp, err := svc.UploadAttachment(ctx, creatorID, taskID, "report.pdf", 1024, "application/pdf", strings.NewReader("%PDF-1.4\n"+strings.Repeat("a", 1015)))
		require.NoError(t, err)
		assert.Equal(t, "report.pdf", resp.FileName)
		assert.Equal(t, int64(1024), resp.FileSize)
		assert.Equal(t, "application/pdf", resp.ContentType)
	})

	t.Run("EmptyFile", func(t *testing.T) {
		svc := NewAttachmentService(&mockAttachmentQuerier{}, &mockStorageClient{}, 10)
		_, err := svc.UploadAttachment(ctx, creatorID, taskID, "file.txt", 0, "text/plain", strings.NewReader(""))
		assert.ErrorIs(t, err, ErrEmptyFile)
	})

	t.Run("FileTooLarge", func(t *testing.T) {
		svc := NewAttachmentService(&mockAttachmentQuerier{}, &mockStorageClient{}, 1) // 1MB max
		_, err := svc.UploadAttachment(ctx, creatorID, taskID, "large.zip", 2*1024*1024, "application/zip", strings.NewReader("PK\x03\x04too big"))
		assert.ErrorIs(t, err, ErrFileTooLarge)
	})

	t.Run("DisallowedFileType_NotInAllowlist", func(t *testing.T) {
		svc := NewAttachmentService(&mockAttachmentQuerier{}, &mockStorageClient{}, 10)
		_, err := svc.UploadAttachment(ctx, creatorID, taskID, "malware.exe", 500, "application/octet-stream", strings.NewReader("bad"))
		assert.ErrorIs(t, err, ErrInvalidFileType)
	})

	t.Run("SpoofedExtension_HTMLInPDF", func(t *testing.T) {
		mockRepo := &mockAttachmentQuerier{
			getTaskByIDFn: func(_ context.Context, _ uuid.UUID) (repository.Task, error) {
				return task, nil
			},
			getProjectByIDFn: func(_ context.Context, _ uuid.UUID) (repository.Project, error) {
				return project, nil
			},
		}
		svc := NewAttachmentService(mockRepo, &mockStorageClient{}, 10)
		_, err := svc.UploadAttachment(ctx, creatorID, taskID, "contract.pdf", 50, "application/pdf", strings.NewReader("<html><script>alert(1)</script></html>"))
		assert.ErrorIs(t, err, ErrInvalidFileType)
	})

	t.Run("SpoofedExtension_HTMLInPNG", func(t *testing.T) {
		mockRepo := &mockAttachmentQuerier{
			getTaskByIDFn: func(_ context.Context, _ uuid.UUID) (repository.Task, error) {
				return task, nil
			},
			getProjectByIDFn: func(_ context.Context, _ uuid.UUID) (repository.Project, error) {
				return project, nil
			},
		}
		svc := NewAttachmentService(mockRepo, &mockStorageClient{}, 10)
		_, err := svc.UploadAttachment(ctx, creatorID, taskID, "photo.png", 50, "image/png", strings.NewReader("<html><body>malicious</body></html>"))
		assert.ErrorIs(t, err, ErrInvalidFileType)
	})

	t.Run("TaskNotFound_AntiIDOR", func(t *testing.T) {
		mockRepo := &mockAttachmentQuerier{
			getTaskByIDFn: func(_ context.Context, _ uuid.UUID) (repository.Task, error) {
				return repository.Task{}, pgx.ErrNoRows
			},
		}
		svc := NewAttachmentService(mockRepo, &mockStorageClient{}, 10)
		_, err := svc.UploadAttachment(ctx, creatorID, uuid.New(), "doc.pdf", 100, "application/pdf", strings.NewReader("%PDF-1.4 content"))
		assert.ErrorIs(t, err, ErrTaskNotFound)
	})

	t.Run("Forbidden_ViewerCannotUpload", func(t *testing.T) {
		mockRepo := &mockAttachmentQuerier{
			getTaskByIDFn: func(_ context.Context, _ uuid.UUID) (repository.Task, error) {
				return task, nil
			},
			getProjectByIDFn: func(_ context.Context, _ uuid.UUID) (repository.Project, error) {
				return project, nil
			},
			getProjectMemberFn: func(_ context.Context, arg repository.GetProjectMemberParams) (repository.ProjectMember, error) {
				if arg.UserID == viewerID {
					return repository.ProjectMember{Role: "viewer"}, nil
				}
				return repository.ProjectMember{}, pgx.ErrNoRows
			},
		}
		svc := NewAttachmentService(mockRepo, &mockStorageClient{}, 10)
		_, err := svc.UploadAttachment(ctx, viewerID, taskID, "doc.pdf", 100, "application/pdf", strings.NewReader("%PDF-1.4 content"))
		assert.ErrorIs(t, err, ErrInsufficientProjectRole)
	})
}

func TestAttachmentService_ListAttachments(t *testing.T) {
	ctx := context.Background()
	taskID := uuid.New()
	userID := uuid.New()

	t.Run("Success", func(t *testing.T) {
		mockRepo := &mockAttachmentQuerier{
			getTaskByIDFn: func(_ context.Context, _ uuid.UUID) (repository.Task, error) {
				return repository.Task{ID: taskID, CreatorID: userID}, nil
			},
			listTaskAttachmentsByTaskIDFn: func(_ context.Context, id uuid.UUID) ([]repository.TaskAttachment, error) {
				assert.Equal(t, taskID, id)
				return []repository.TaskAttachment{
					{
						ID:          uuid.New(),
						TaskID:      taskID,
						UserID:      userID,
						FileName:    "spec.pdf",
						FileSize:    2048,
						ContentType: "application/pdf",
						CreatedAt:   pgtype.Timestamptz{Time: time.Now(), Valid: true},
					},
				}, nil
			},
		}
		svc := NewAttachmentService(mockRepo, &mockStorageClient{}, 10)
		attachments, err := svc.ListAttachments(ctx, userID, taskID)
		require.NoError(t, err)
		assert.Len(t, attachments, 1)
		assert.Equal(t, "spec.pdf", attachments[0].FileName)
	})

	t.Run("TaskNotFound_AntiIDOR", func(t *testing.T) {
		mockRepo := &mockAttachmentQuerier{
			getTaskByIDFn: func(_ context.Context, _ uuid.UUID) (repository.Task, error) {
				return repository.Task{}, pgx.ErrNoRows
			},
		}
		svc := NewAttachmentService(mockRepo, &mockStorageClient{}, 10)
		_, err := svc.ListAttachments(ctx, userID, taskID)
		assert.ErrorIs(t, err, ErrTaskNotFound)
	})
}

func TestAttachmentService_GetAttachment(t *testing.T) {
	ctx := context.Background()
	taskID := uuid.New()
	userID := uuid.New()
	attachmentID := uuid.New()

	t.Run("Success", func(t *testing.T) {
		mockRepo := &mockAttachmentQuerier{
			getTaskByIDFn: func(_ context.Context, _ uuid.UUID) (repository.Task, error) {
				return repository.Task{ID: taskID, CreatorID: userID}, nil
			},
			getTaskAttachmentByIDFn: func(_ context.Context, id uuid.UUID) (repository.TaskAttachment, error) {
				if id == attachmentID {
					return repository.TaskAttachment{
						ID:          attachmentID,
						TaskID:      taskID,
						UserID:      userID,
						FileName:    "image.png",
						FileSize:    4096,
						ContentType: "image/png",
						StorageKey:  "tasks/" + taskID.String() + "/image.png",
						CreatedAt:   pgtype.Timestamptz{Time: time.Now(), Valid: true},
					}, nil
				}
				return repository.TaskAttachment{}, pgx.ErrNoRows
			},
		}
		mockStore := &mockStorageClient{
			downloadFunc: func(_ context.Context, _ string) (io.ReadCloser, error) {
				return io.NopCloser(strings.NewReader("png binary stream")), nil
			},
		}
		svc := NewAttachmentService(mockRepo, mockStore, 10)

		att, stream, err := svc.GetAttachment(ctx, userID, taskID, attachmentID)
		require.NoError(t, err)
		defer stream.Close()

		assert.Equal(t, "image.png", att.FileName)
		content, _ := io.ReadAll(stream)
		assert.Equal(t, "png binary stream", string(content))
	})

	t.Run("AttachmentNotFound", func(t *testing.T) {
		mockRepo := &mockAttachmentQuerier{
			getTaskByIDFn: func(_ context.Context, _ uuid.UUID) (repository.Task, error) {
				return repository.Task{ID: taskID, CreatorID: userID}, nil
			},
			getTaskAttachmentByIDFn: func(_ context.Context, _ uuid.UUID) (repository.TaskAttachment, error) {
				return repository.TaskAttachment{}, pgx.ErrNoRows
			},
		}
		svc := NewAttachmentService(mockRepo, &mockStorageClient{}, 10)
		_, _, err := svc.GetAttachment(ctx, userID, taskID, attachmentID)
		assert.ErrorIs(t, err, ErrAttachmentNotFound)
	})
}

func TestAttachmentService_DeleteAttachment(t *testing.T) {
	ctx := context.Background()
	taskID := uuid.New()
	creatorID := uuid.New()
	uploaderID := uuid.New()
	otherUserID := uuid.New()
	attachmentID := uuid.New()

	task := repository.Task{
		ID:        taskID,
		CreatorID: creatorID,
	}
	attachment := repository.TaskAttachment{
		ID:         attachmentID,
		TaskID:     taskID,
		UserID:     uploaderID,
		StorageKey: "tasks/" + taskID.String() + "/file.pdf",
	}

	t.Run("AuthorDeletes_Success", func(t *testing.T) {
		deletedStorageKey := ""
		deletedDBID := uuid.Nil

		taskWithAssignee := task
		taskWithAssignee.AssigneeID = pgtype.UUID{Bytes: uploaderID, Valid: true}

		mockRepo := &mockAttachmentQuerier{
			getTaskByIDFn: func(_ context.Context, _ uuid.UUID) (repository.Task, error) {
				return taskWithAssignee, nil
			},
			getTaskAttachmentByIDFn: func(_ context.Context, _ uuid.UUID) (repository.TaskAttachment, error) {
				return attachment, nil
			},
			deleteTaskAttachmentFn: func(_ context.Context, id uuid.UUID) error {
				deletedDBID = id
				return nil
			},
		}
		mockStore := &mockStorageClient{
			deleteFunc: func(_ context.Context, key string) error {
				deletedStorageKey = key
				return nil
			},
		}
		svc := NewAttachmentService(mockRepo, mockStore, 10)

		err := svc.DeleteAttachment(ctx, uploaderID, taskID, attachmentID)
		require.NoError(t, err)
		assert.Equal(t, attachmentID, deletedDBID)
		assert.Equal(t, attachment.StorageKey, deletedStorageKey)
	})

	t.Run("TaskCreatorModerates_Success", func(t *testing.T) {
		mockRepo := &mockAttachmentQuerier{
			getTaskByIDFn: func(_ context.Context, _ uuid.UUID) (repository.Task, error) {
				return task, nil
			},
			getTaskAttachmentByIDFn: func(_ context.Context, _ uuid.UUID) (repository.TaskAttachment, error) {
				return attachment, nil
			},
			deleteTaskAttachmentFn: func(_ context.Context, _ uuid.UUID) error {
				return nil
			},
		}
		svc := NewAttachmentService(mockRepo, &mockStorageClient{}, 10)
		err := svc.DeleteAttachment(ctx, creatorID, taskID, attachmentID)
		require.NoError(t, err)
	})

	t.Run("Forbidden_RandomUser", func(t *testing.T) {
		mockRepo := &mockAttachmentQuerier{
			getTaskByIDFn: func(_ context.Context, _ uuid.UUID) (repository.Task, error) {
				// Give user access via AssigneeID so they can view task, but are neither author nor creator
				t := task
				t.AssigneeID = pgtype.UUID{Bytes: otherUserID, Valid: true}
				return t, nil
			},
			getTaskAttachmentByIDFn: func(_ context.Context, _ uuid.UUID) (repository.TaskAttachment, error) {
				return attachment, nil
			},
		}
		svc := NewAttachmentService(mockRepo, &mockStorageClient{}, 10)
		err := svc.DeleteAttachment(ctx, otherUserID, taskID, attachmentID)
		assert.ErrorIs(t, err, ErrForbidden)
	})
}
