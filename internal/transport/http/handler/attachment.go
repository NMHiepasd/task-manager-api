package handler

import (
	"fmt"
	"net/http"
	"strconv"
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
}

// NewAttachmentHandler creates a new AttachmentHandler instance.
func NewAttachmentHandler(attachmentService service.AttachmentService) *AttachmentHandler {
	return &AttachmentHandler{
		attachmentService: attachmentService,
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

	if c.Query("presigned") == "true" {
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

	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", att.FileName))
	c.Header("Content-Type", att.ContentType)
	c.Header("Content-Length", strconv.FormatInt(att.FileSize, 10))

	c.DataFromReader(http.StatusOK, att.FileSize, att.ContentType, stream, nil)
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
