package handler

import (
	"fmt"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"task-manager-api/internal/domain"
	"task-manager-api/internal/service"
	"task-manager-api/internal/transport/http/middleware"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// AttachmentHandler handles HTTP requests for task file attachments.
type AttachmentHandler struct {
	attachmentService service.AttachmentService
	presignEnabled    bool
}

// NewAttachmentHandler creates a new AttachmentHandler instance.
// Optional presignEnabled controls whether ?presigned=true generates temporary URLs or streams directly (default true).
func NewAttachmentHandler(attachmentService service.AttachmentService, presignEnabled ...bool) *AttachmentHandler {
	enabled := true
	if len(presignEnabled) > 0 {
		enabled = presignEnabled[0]
	}
	return &AttachmentHandler{
		attachmentService: attachmentService,
		presignEnabled:    enabled,
	}
}

// Upload handles multipart file upload for a task.
// @Summary      Upload task attachment
// @Description  Uploads a file attachment to a task. Caller must have write access (Owner or Editor) to the task/project.
// @Tags         attachments
// @Accept       multipart/form-data
// @Produce      json
// @Produce      application/problem+json
// @Security     BearerAuth
// @Param        id path string true "Task UUID"
// @Param        file formData file true "File to upload"
// @Success      201 {object} service.AttachmentResponse
// @Failure      400 {object} ProblemDetails
// @Failure      401 {object} ProblemDetails
// @Failure      403 {object} ProblemDetails
// @Failure      404 {object} ProblemDetails
// @Failure      413 {object} ProblemDetails
// @Router       /api/v1/tasks/{id}/attachments [post]
func (h *AttachmentHandler) Upload(c *gin.Context) {
	userID, err := middleware.GetAuthUserID(c)
	if err != nil {
		_ = c.Error(err)
		return
	}

	taskID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		_ = c.Error(domain.ValidationErrors{
			Message: "Invalid UUID format in path parameter",
			Params:  []domain.InvalidParam{{Name: "id", Reason: "must be a valid UUID"}},
		})
		return
	}

	fileHeader, err := c.FormFile("file")
	if err != nil {
		_ = c.Error(domain.ValidationErrors{
			Message: "Missing or invalid file multipart field",
			Params:  []domain.InvalidParam{{Name: "file", Reason: "file field is required in multipart form data"}},
		})
		return
	}

	file, err := fileHeader.Open()
	if err != nil {
		_ = c.Error(fmt.Errorf("failed to open uploaded file: %w", err))
		return
	}
	defer file.Close()

	contentType := fileHeader.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	resp, err := h.attachmentService.UploadAttachment(
		c.Request.Context(),
		userID,
		taskID,
		fileHeader.Filename,
		fileHeader.Size,
		contentType,
		file,
	)
	if err != nil {
		_ = c.Error(err)
		return
	}

	c.JSON(http.StatusCreated, resp)
}

// List retrieves all attachments belonging to a task.
// @Summary      List task attachments
// @Description  Lists all attachments attached to the given task. Caller must have read access.
// @Tags         attachments
// @Produce      json
// @Produce      application/problem+json
// @Security     BearerAuth
// @Param        id path string true "Task UUID"
// @Success      200 {array} service.AttachmentResponse
// @Failure      401 {object} ProblemDetails
// @Failure      404 {object} ProblemDetails
// @Router       /api/v1/tasks/{id}/attachments [get]
func (h *AttachmentHandler) List(c *gin.Context) {
	userID, err := middleware.GetAuthUserID(c)
	if err != nil {
		_ = c.Error(err)
		return
	}

	taskID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		_ = c.Error(domain.ValidationErrors{
			Message: "Invalid UUID format in path parameter",
			Params:  []domain.InvalidParam{{Name: "id", Reason: "must be a valid UUID"}},
		})
		return
	}

	attachments, err := h.attachmentService.ListAttachments(c.Request.Context(), userID, taskID)
	if err != nil {
		_ = c.Error(err)
		return
	}

	c.JSON(http.StatusOK, attachments)
}

// Download streams the file attachment or returns a presigned download URL.
// @Summary      Download task attachment
// @Description  Streams attachment content or returns a temporary presigned URL if ?presigned=true.
// @Tags         attachments
// @Produce      application/octet-stream
// @Produce      json
// @Produce      application/problem+json
// @Security     BearerAuth
// @Param        id path string true "Task UUID"
// @Param        attachmentId path string true "Attachment UUID"
// @Param        presigned query bool false "Return temporary presigned URL instead of streaming"
// @Success      200 {file} binary "Attachment file stream"
// @Failure      401 {object} ProblemDetails
// @Failure      404 {object} ProblemDetails
// @Router       /api/v1/tasks/{id}/attachments/{attachmentId} [get]
func (h *AttachmentHandler) Download(c *gin.Context) {
	userID, err := middleware.GetAuthUserID(c)
	if err != nil {
		_ = c.Error(err)
		return
	}

	taskID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		_ = c.Error(domain.ValidationErrors{
			Message: "Invalid UUID format in path parameter",
			Params:  []domain.InvalidParam{{Name: "id", Reason: "must be a valid UUID"}},
		})
		return
	}

	attachmentID, err := uuid.Parse(c.Param("attachmentId"))
	if err != nil {
		_ = c.Error(domain.ValidationErrors{
			Message: "Invalid UUID format in path parameter",
			Params:  []domain.InvalidParam{{Name: "attachmentId", Reason: "must be a valid UUID"}},
		})
		return
	}

	if h.presignEnabled && c.Query("presigned") == "true" {
		presignedURL, err := h.attachmentService.GetAttachmentPresignedURL(
			c.Request.Context(),
			userID,
			taskID,
			attachmentID,
			15*time.Minute,
		)
		if err != nil {
			_ = c.Error(err)
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"download_url": presignedURL,
			"expires_in":   900,
		})
		return
	}

	att, stream, err := h.attachmentService.GetAttachment(c.Request.Context(), userID, taskID, attachmentID)
	if err != nil {
		_ = c.Error(err)
		return
	}
	defer stream.Close()

	c.Header("Content-Disposition", formatContentDisposition(att.FileName))
	c.Header("Content-Type", att.ContentType)
	c.Header("Content-Length", strconv.FormatInt(att.FileSize, 10))
	c.Header("X-Content-Type-Options", "nosniff")

	c.DataFromReader(http.StatusOK, att.FileSize, att.ContentType, stream, nil)
}

// formatContentDisposition securely formats the Content-Disposition header conforming to RFC 2616 / RFC 6266.
// It escapes quotes, strips CRLF injection vectors, and encodes Unicode filenames with RFC 5987.
func formatContentDisposition(filename string) string {
	// Strip CRLF to prevent HTTP response splitting
	cleanName := strings.ReplaceAll(filename, "\r", "")
	cleanName = strings.ReplaceAll(cleanName, "\n", "")

	disposition := mime.FormatMediaType("attachment", map[string]string{
		"filename": cleanName,
	})
	if disposition != "" {
		return disposition
	}

	// For Unicode or special characters where basic FormatMediaType returns empty,
	// provide an ASCII fallback along with RFC 5987 UTF-8 encoded parameter.
	asciiFallback := toASCIIFallback(cleanName)
	encodedUTF8 := "UTF-8''" + url.PathEscape(cleanName)

	disposition = mime.FormatMediaType("attachment", map[string]string{
		"filename":  asciiFallback,
		"filename*": encodedUTF8,
	})
	if disposition != "" {
		return disposition
	}

	return fmt.Sprintf("attachment; filename=\"%s\"; filename*=%s", asciiFallback, encodedUTF8)
}

func toASCIIFallback(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= 0x20 && r <= 0x7e && r != '"' && r != '\\' {
			b.WriteRune(r)
		} else {
			b.WriteRune('_')
		}
	}
	res := strings.TrimSpace(b.String())
	if res == "" || res == "_" {
		return "attachment"
	}
	return res
}

// Delete removes a task attachment.
// @Summary      Delete task attachment
// @Description  Removes an attachment from a task. Caller must be the uploader, task creator, or project owner.
// @Tags         attachments
// @Produce      json
// @Produce      application/problem+json
// @Security     BearerAuth
// @Param        id path string true "Task UUID"
// @Param        attachmentId path string true "Attachment UUID"
// @Success      204 "Attachment deleted"
// @Failure      401 {object} ProblemDetails
// @Failure      403 {object} ProblemDetails
// @Failure      404 {object} ProblemDetails
// @Router       /api/v1/tasks/{id}/attachments/{attachmentId} [delete]
func (h *AttachmentHandler) Delete(c *gin.Context) {
	userID, err := middleware.GetAuthUserID(c)
	if err != nil {
		_ = c.Error(err)
		return
	}

	taskID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		_ = c.Error(domain.ValidationErrors{
			Message: "Invalid UUID format in path parameter",
			Params:  []domain.InvalidParam{{Name: "id", Reason: "must be a valid UUID"}},
		})
		return
	}

	attachmentID, err := uuid.Parse(c.Param("attachmentId"))
	if err != nil {
		_ = c.Error(domain.ValidationErrors{
			Message: "Invalid UUID format in path parameter",
			Params:  []domain.InvalidParam{{Name: "attachmentId", Reason: "must be a valid UUID"}},
		})
		return
	}

	if err := h.attachmentService.DeleteAttachment(c.Request.Context(), userID, taskID, attachmentID); err != nil {
		_ = c.Error(err)
		return
	}

	c.Status(http.StatusNoContent)
}
