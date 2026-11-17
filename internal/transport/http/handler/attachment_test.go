package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"task-manager-api/internal/service"
	"task-manager-api/internal/transport/http/middleware"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockAttachmentService struct {
	uploadAttachmentFn          func(ctx context.Context, userID, taskID uuid.UUID, fileName string, fileSize int64, contentType string, reader io.Reader) (*service.AttachmentResponse, error)
	listAttachmentsFn           func(ctx context.Context, userID, taskID uuid.UUID) ([]service.AttachmentResponse, error)
	getAttachmentFn             func(ctx context.Context, userID, taskID, attachmentID uuid.UUID) (*service.AttachmentResponse, io.ReadCloser, error)
	getAttachmentPresignedURLFn func(ctx context.Context, userID, taskID, attachmentID uuid.UUID, expiry time.Duration) (string, error)
	deleteAttachmentFn          func(ctx context.Context, userID, taskID, attachmentID uuid.UUID) error
}

func (m *mockAttachmentService) UploadAttachment(ctx context.Context, userID, taskID uuid.UUID, fileName string, fileSize int64, contentType string, reader io.Reader) (*service.AttachmentResponse, error) {
	if m.uploadAttachmentFn != nil {
		return m.uploadAttachmentFn(ctx, userID, taskID, fileName, fileSize, contentType, reader)
	}
	return nil, nil
}

func (m *mockAttachmentService) ListAttachments(ctx context.Context, userID, taskID uuid.UUID) ([]service.AttachmentResponse, error) {
	if m.listAttachmentsFn != nil {
		return m.listAttachmentsFn(ctx, userID, taskID)
	}
	return nil, nil
}

func (m *mockAttachmentService) GetAttachment(ctx context.Context, userID, taskID, attachmentID uuid.UUID) (*service.AttachmentResponse, io.ReadCloser, error) {
	if m.getAttachmentFn != nil {
		return m.getAttachmentFn(ctx, userID, taskID, attachmentID)
	}
	return nil, nil, nil
}

func (m *mockAttachmentService) GetAttachmentPresignedURL(ctx context.Context, userID, taskID, attachmentID uuid.UUID, expiry time.Duration) (string, error) {
	if m.getAttachmentPresignedURLFn != nil {
		return m.getAttachmentPresignedURLFn(ctx, userID, taskID, attachmentID, expiry)
	}
	return "", nil
}

func (m *mockAttachmentService) DeleteAttachment(ctx context.Context, userID, taskID, attachmentID uuid.UUID) error {
	if m.deleteAttachmentFn != nil {
		return m.deleteAttachmentFn(ctx, userID, taskID, attachmentID)
	}
	return nil
}

func setupTestAttachmentRouter(handler *AttachmentHandler, userID uuid.UUID) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(middleware.ErrorHandler(nil))
	router.Use(func(c *gin.Context) {
		c.Set(middleware.AuthorizationUserIDKey, userID)
		c.Next()
	})

	api := router.Group("/api/v1/tasks/:id/attachments")
	{
		api.POST("", handler.Upload)
		api.GET("", handler.List)
		api.GET("/:attachmentId", handler.Download)
		api.DELETE("/:attachmentId", handler.Delete)
	}

	return router
}

func TestAttachmentHandler_Upload(t *testing.T) {
	userID := uuid.New()
	taskID := uuid.New()

	t.Run("Created_201", func(t *testing.T) {
		svc := &mockAttachmentService{
			uploadAttachmentFn: func(_ context.Context, uid, tid uuid.UUID, fileName string, fileSize int64, contentType string, _ io.Reader) (*service.AttachmentResponse, error) {
				assert.Equal(t, userID, uid)
				assert.Equal(t, taskID, tid)
				assert.Equal(t, "test.txt", fileName)
				assert.Equal(t, int64(12), fileSize)
				return &service.AttachmentResponse{
					ID:          uuid.New(),
					TaskID:      tid,
					UserID:      uid,
					FileName:    fileName,
					FileSize:    fileSize,
					ContentType: contentType,
					CreatedAt:   time.Now(),
				}, nil
			},
		}
		handler := NewAttachmentHandler(svc)
		router := setupTestAttachmentRouter(handler, userID)

		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)
		part, err := writer.CreateFormFile("file", "test.txt")
		require.NoError(t, err)
		_, _ = part.Write([]byte("hello world!"))
		require.NoError(t, writer.Close())

		req := httptest.NewRequest(http.MethodPost, "/api/v1/tasks/"+taskID.String()+"/attachments", body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusCreated, w.Code)

		var resp service.AttachmentResponse
		err = json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, "test.txt", resp.FileName)
	})

	t.Run("BadRequest_MissingFile", func(t *testing.T) {
		handler := NewAttachmentHandler(&mockAttachmentService{})
		router := setupTestAttachmentRouter(handler, userID)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/tasks/"+taskID.String()+"/attachments", strings.NewReader(""))
		req.Header.Set("Content-Type", "multipart/form-data")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestAttachmentHandler_List(t *testing.T) {
	userID := uuid.New()
	taskID := uuid.New()

	svc := &mockAttachmentService{
		listAttachmentsFn: func(_ context.Context, uid, tid uuid.UUID) ([]service.AttachmentResponse, error) {
			assert.Equal(t, userID, uid)
			assert.Equal(t, taskID, tid)
			return []service.AttachmentResponse{
				{
					ID:          uuid.New(),
					TaskID:      tid,
					UserID:      uid,
					FileName:    "image.png",
					FileSize:    1024,
					ContentType: "image/png",
					CreatedAt:   time.Now(),
				},
			}, nil
		},
	}
	handler := NewAttachmentHandler(svc)
	router := setupTestAttachmentRouter(handler, userID)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/tasks/"+taskID.String()+"/attachments", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	var resp []service.AttachmentResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Len(t, resp, 1)
	assert.Equal(t, "image.png", resp[0].FileName)
}

func TestAttachmentHandler_Download(t *testing.T) {
	userID := uuid.New()
	taskID := uuid.New()
	attachmentID := uuid.New()

	t.Run("Stream_200", func(t *testing.T) {
		svc := &mockAttachmentService{
			getAttachmentFn: func(_ context.Context, uid, tid, aid uuid.UUID) (*service.AttachmentResponse, io.ReadCloser, error) {
				return &service.AttachmentResponse{
					ID:          aid,
					TaskID:      tid,
					UserID:      uid,
					FileName:    "report.pdf",
					FileSize:    14,
					ContentType: "application/pdf",
					CreatedAt:   time.Now(),
				}, io.NopCloser(strings.NewReader("PDF stream data")), nil
			},
		}
		handler := NewAttachmentHandler(svc)
		router := setupTestAttachmentRouter(handler, userID)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/tasks/"+taskID.String()+"/attachments/"+attachmentID.String(), nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "PDF stream data", w.Body.String())
		assert.Contains(t, w.Header().Get("Content-Disposition"), "report.pdf")
		assert.Equal(t, "application/pdf", w.Header().Get("Content-Type"))
	})

	t.Run("Presigned_200", func(t *testing.T) {
		svc := &mockAttachmentService{
			getAttachmentPresignedURLFn: func(_ context.Context, _, _, _ uuid.UUID, _ time.Duration) (string, error) {
				return "https://storage.local/presigned-url?sig=abc", nil
			},
		}
		handler := NewAttachmentHandler(svc)
		router := setupTestAttachmentRouter(handler, userID)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/tasks/"+taskID.String()+"/attachments/"+attachmentID.String()+"?presigned=true", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)

		var resp map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, "https://storage.local/presigned-url?sig=abc", resp["download_url"])
	})

	t.Run("PresignDisabled_FallbackToStream", func(t *testing.T) {
		svc := &mockAttachmentService{
			getAttachmentFn: func(_ context.Context, _, _, _ uuid.UUID) (*service.AttachmentResponse, io.ReadCloser, error) {
				return &service.AttachmentResponse{
					ID:          attachmentID,
					TaskID:      taskID,
					UserID:      userID,
					FileName:    "fallback.pdf",
					FileSize:    1024,
					ContentType: "application/pdf",
					CreatedAt:   time.Now(),
				}, io.NopCloser(strings.NewReader("Streamed fallback data")), nil
			},
		}
		// Presign disabled
		handler := NewAttachmentHandler(svc, false)
		router := setupTestAttachmentRouter(handler, userID)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/tasks/"+taskID.String()+"/attachments/"+attachmentID.String()+"?presigned=true", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "Streamed fallback data", w.Body.String())
		assert.Contains(t, w.Header().Get("Content-Disposition"), "fallback.pdf")
		assert.Equal(t, "application/pdf", w.Header().Get("Content-Type"))
	})
}

func TestAttachmentHandler_Delete(t *testing.T) {
	userID := uuid.New()
	taskID := uuid.New()
	attachmentID := uuid.New()

	svc := &mockAttachmentService{
		deleteAttachmentFn: func(_ context.Context, uid, tid, aid uuid.UUID) error {
			assert.Equal(t, userID, uid)
			assert.Equal(t, taskID, tid)
			assert.Equal(t, attachmentID, aid)
			return nil
		},
	}
	handler := NewAttachmentHandler(svc)
	router := setupTestAttachmentRouter(handler, userID)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/tasks/"+taskID.String()+"/attachments/"+attachmentID.String(), nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNoContent, w.Code)
}

func TestAttachmentHandler_Download_SafeContentDisposition(t *testing.T) {
	userID := uuid.New()
	taskID := uuid.New()
	attachmentID := uuid.New()

	tests := []struct {
		name                   string
		filename               string
		expectedDisposContains []string
		prohibitedDispos       []string
	}{
		{
			name:     "DoubleQuotesEscaped",
			filename: `my "special" document.pdf`,
			expectedDisposContains: []string{
				`attachment;`,
				`filename="my \"special\" document.pdf"`,
			},
			prohibitedDispos: []string{
				`filename="my "special"`,
			},
		},
		{
			name:     "NewlinesStripped",
			filename: "malicious\r\nheader\ninjection.pdf",
			expectedDisposContains: []string{
				`attachment;`,
				`maliciousheaderinjection.pdf`,
			},
			prohibitedDispos: []string{
				"\r",
				"\n",
			},
		},
		{
			name:     "UnicodeEncoded",
			filename: "báo_cáo_tài_chính.pdf",
			expectedDisposContains: []string{
				`attachment;`,
				`filename*=`,
				`b%C3%A1o_c%C3%A1o_t%C3%A0i_ch%C3%ADnh.pdf`,
			},
			prohibitedDispos: []string{
				"\r",
				"\n",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := &mockAttachmentService{
				getAttachmentFn: func(_ context.Context, _, _, _ uuid.UUID) (*service.AttachmentResponse, io.ReadCloser, error) {
					return &service.AttachmentResponse{
						ID:          attachmentID,
						TaskID:      taskID,
						UserID:      userID,
						FileName:    tc.filename,
						FileSize:    100,
						ContentType: "application/pdf",
						CreatedAt:   time.Now(),
					}, io.NopCloser(strings.NewReader("dummy stream")), nil
				},
			}
			handler := NewAttachmentHandler(svc)
			router := setupTestAttachmentRouter(handler, userID)

			req := httptest.NewRequest(http.MethodGet, "/api/v1/tasks/"+taskID.String()+"/attachments/"+attachmentID.String(), nil)
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)
			assert.Equal(t, http.StatusOK, w.Code)
			assert.Equal(t, "nosniff", w.Header().Get("X-Content-Type-Options"))

			disposition := w.Header().Get("Content-Disposition")
			for _, expected := range tc.expectedDisposContains {
				assert.Contains(t, disposition, expected)
			}
			for _, prohibited := range tc.prohibitedDispos {
				assert.NotContains(t, disposition, prohibited)
			}
		})
	}
}
