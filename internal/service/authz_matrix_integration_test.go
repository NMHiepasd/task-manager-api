//go:build integration

package service_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"task-manager-api/internal/cache"
	"task-manager-api/internal/repository"
	"task-manager-api/internal/service"
)

var (
	testStore repository.Store
	testPool  *pgxpool.Pool
)

// inMemoryStorage implements storage.Client for integration tests without MinIO.
type inMemoryStorage struct{}

func (s *inMemoryStorage) Upload(_ context.Context, _ string, _ io.Reader, _ int64, _ string) error {
	return nil
}

func (s *inMemoryStorage) Download(_ context.Context, _ string) (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader([]byte("dummy content"))), nil
}

func (s *inMemoryStorage) Delete(_ context.Context, _ string) error {
	return nil
}

func (s *inMemoryStorage) GetPresignedURL(_ context.Context, key string, _ time.Duration) (string, error) {
	return "https://storage.local/" + key, nil
}

func (s *inMemoryStorage) EnsureBucket(_ context.Context) error {
	return nil
}

func TestMain(m *testing.M) {
	ctx := context.Background()

	pgContainer, err := postgres.Run(ctx,
		"postgres:16-alpine",
		postgres.WithDatabase("test_task_manager_authz"),
		postgres.WithUsername("postgres"),
		postgres.WithPassword("postgres"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(60*time.Second),
		),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to start postgres testcontainer: %v\n", err)
		os.Exit(1)
	}

	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to get connection string: %v\n", err)
		_ = pgContainer.Terminate(ctx)
		os.Exit(1)
	}

	config, err := pgxpool.ParseConfig(connStr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to parse pool config: %v\n", err)
		_ = pgContainer.Terminate(ctx)
		os.Exit(1)
	}

	testPool, err = pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to connect pool: %v\n", err)
		_ = pgContainer.Terminate(ctx)
		os.Exit(1)
	}

	migrationFiles, err := filepath.Glob(filepath.Join("..", "..", "migrations", "*.up.sql"))
	if err != nil || len(migrationFiles) == 0 {
		fmt.Fprintf(os.Stderr, "failed to find migrations: %v\n", err)
		_ = pgContainer.Terminate(ctx)
		os.Exit(1)
	}
	sort.Strings(migrationFiles)

	for _, file := range migrationFiles {
		sqlBytes, err := os.ReadFile(file)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to read migration file %s: %v\n", file, err)
			testPool.Close()
			_ = pgContainer.Terminate(ctx)
			os.Exit(1)
		}

		if _, err := testPool.Exec(ctx, string(sqlBytes)); err != nil {
			fmt.Fprintf(os.Stderr, "failed to execute migration %s: %v\n", file, err)
			testPool.Close()
			_ = pgContainer.Terminate(ctx)
			os.Exit(1)
		}
	}

	testStore = repository.NewStore(testPool)

	code := m.Run()

	testPool.Close()
	_ = pgContainer.Terminate(ctx)

	os.Exit(code)
}

func createTestUser(ctx context.Context, t *testing.T, emailPrefix string) repository.User {
	t.Helper()
	email := fmt.Sprintf("%s_%d@example.com", emailPrefix, time.Now().UnixNano())
	user, err := testStore.CreateUser(ctx, repository.CreateUserParams{
		Email:        email,
		PasswordHash: "hashed_secret_123",
		FullName:     emailPrefix,
		Role:         "user",
	})
	require.NoError(t, err)
	return user
}

func createTestProject(ctx context.Context, t *testing.T, ownerID uuid.UUID, name string) repository.Project {
	t.Helper()
	proj, err := testStore.CreateProject(ctx, repository.CreateProjectParams{
		OwnerID:     ownerID,
		Name:        name,
		Description: "Integration Test Project",
		Color:       "#3b82f6",
	})
	require.NoError(t, err)
	return proj
}

func addTestMember(ctx context.Context, t *testing.T, projectID, userID uuid.UUID, role string) {
	t.Helper()
	_, err := testStore.AddProjectMember(ctx, repository.AddProjectMemberParams{
		ProjectID: projectID,
		UserID:    userID,
		Role:      role,
	})
	require.NoError(t, err)
}

func createTestTask(ctx context.Context, t *testing.T, creatorID uuid.UUID, projectID *uuid.UUID, assigneeID *uuid.UUID, title string) repository.Task {
	t.Helper()
	var pID pgtype.UUID
	if projectID != nil {
		pID = pgtype.UUID{Bytes: *projectID, Valid: true}
	}
	var aID pgtype.UUID
	if assigneeID != nil {
		aID = pgtype.UUID{Bytes: *assigneeID, Valid: true}
	}
	task, err := testStore.CreateTask(ctx, repository.CreateTaskParams{
		CreatorID:   creatorID,
		ProjectID:   pID,
		AssigneeID:  aID,
		Title:       title,
		Description: "Task for authz matrix test",
		Status:      "todo",
		Priority:    "medium",
	})
	require.NoError(t, err)
	return task
}

func createTestComment(ctx context.Context, t *testing.T, taskID, authorID uuid.UUID, content string) repository.TaskComment {
	t.Helper()
	comment, err := testStore.CreateTaskComment(ctx, repository.CreateTaskCommentParams{
		TaskID:  taskID,
		UserID:  authorID,
		Content: content,
	})
	require.NoError(t, err)
	return comment
}

func createTestAttachment(ctx context.Context, t *testing.T, taskID, uploaderID uuid.UUID, filename string) repository.TaskAttachment {
	t.Helper()
	storageKey := fmt.Sprintf("tasks/%s/%s-%s", taskID, uuid.New(), filename)
	att, err := testStore.CreateTaskAttachment(ctx, repository.CreateTaskAttachmentParams{
		TaskID:      taskID,
		UserID:      uploaderID,
		FileName:    filename,
		FileSize:    1024,
		ContentType: "text/plain",
		StorageKey:  storageKey,
	})
	require.NoError(t, err)
	return att
}

type authzAction string

const (
	actionRead                authzAction = "Read"
	actionComment             authzAction = "Comment"
	actionEdit                authzAction = "Edit"
	actionDelete              authzAction = "Delete"
	actionUpload              authzAction = "Upload"
	actionModerateComments    authzAction = "ModerateComments"
	actionModerateAttachments authzAction = "ModerateAttachments"
)

type resourceState string

const (
	stateNormal             resourceState = "normal"
	stateTaskSoftDeleted    resourceState = "task_soft_deleted"
	stateProjectSoftDeleted resourceState = "project_soft_deleted"
)

type userRole string

const (
	roleOwner             userRole = "owner"
	roleEditor            userRole = "editor"
	roleViewer            userRole = "viewer"
	roleRemovedMember     userRole = "removed_member"
	roleCreatorNotMember  userRole = "creator_not_member"
	roleAssigneeNotMember userRole = "assignee_not_member"
	roleOutsider          userRole = "outsider"
)

// expectedOutcome defines whether an action is expected to succeed (nil), return 403, or return 404.
type outcome string

const (
	outcomeAllow outcome = "ALLOW"
	outcomeDeny403 outcome = "DENY_403"
	outcomeDeny404 outcome = "DENY_404"
)

func getExpectedOutcome(state resourceState, role userRole, action authzAction) outcome {
	// If task or parent project is soft-deleted, all actions return 404 (anti-IDOR)
	if state == stateTaskSoftDeleted || state == stateProjectSoftDeleted {
		return outcomeDeny404
	}

	// Normal active project state
	switch role {
	case roleOwner:
		return outcomeAllow

	case roleEditor:
		switch action {
		case actionRead, actionComment, actionEdit, actionDelete, actionUpload:
			return outcomeAllow
		case actionModerateComments, actionModerateAttachments:
			return outcomeDeny403
		}

	case roleViewer:
		switch action {
		case actionRead, actionComment:
			return outcomeAllow
		case actionEdit, actionDelete, actionUpload, actionModerateComments, actionModerateAttachments:
			return outcomeDeny403
		}

	case roleRemovedMember, roleCreatorNotMember, roleAssigneeNotMember, roleOutsider:
		return outcomeDeny404
	}

	return outcomeDeny404
}

func TestIntegration_AuthzMatrix(t *testing.T) {
	ctx := context.Background()

	taskService := service.NewTaskService(testStore)
	commentService := service.NewCommentService(testStore)
	attachmentService := service.NewAttachmentService(testStore, &inMemoryStorage{}, 10)
	projectService := service.NewProjectService(testStore, cache.NewNoopCache())

	// Setup users
	ownerUser := createTestUser(ctx, t, "matrix_owner")
	editorUser := createTestUser(ctx, t, "matrix_editor")
	viewerUser := createTestUser(ctx, t, "matrix_viewer")
	removedUser := createTestUser(ctx, t, "matrix_removed")
	creatorNotMemberUser := createTestUser(ctx, t, "matrix_creator_not_member")
	assigneeNotMemberUser := createTestUser(ctx, t, "matrix_assignee_not_member")
	outsiderUser := createTestUser(ctx, t, "matrix_outsider")

	// Setup projects
	normalProject := createTestProject(ctx, t, ownerUser.ID, "Matrix Normal Project")
	addTestMember(ctx, t, normalProject.ID, editorUser.ID, "editor")
	addTestMember(ctx, t, normalProject.ID, viewerUser.ID, "viewer")
	addTestMember(ctx, t, normalProject.ID, removedUser.ID, "editor")
	// Remove removedUser to verify post-removal state
	err := projectService.RemoveMember(ctx, ownerUser.ID, normalProject.ID, removedUser.ID)
	require.NoError(t, err)

	deletedProject := createTestProject(ctx, t, ownerUser.ID, "Matrix Deleted Project")
	addTestMember(ctx, t, deletedProject.ID, editorUser.ID, "editor")
	addTestMember(ctx, t, deletedProject.ID, viewerUser.ID, "viewer")
	// Soft-delete the project
	_, err = testPool.Exec(ctx, "UPDATE projects SET deleted_at = NOW() WHERE id = $1", deletedProject.ID)
	require.NoError(t, err)

	userMap := map[userRole]repository.User{
		roleOwner:             ownerUser,
		roleEditor:            editorUser,
		roleViewer:            viewerUser,
		roleRemovedMember:     removedUser,
		roleCreatorNotMember:  creatorNotMemberUser,
		roleAssigneeNotMember: assigneeNotMemberUser,
		roleOutsider:          outsiderUser,
	}

	states := []resourceState{stateNormal, stateTaskSoftDeleted, stateProjectSoftDeleted}
	roles := []userRole{
		roleOwner,
		roleEditor,
		roleViewer,
		roleRemovedMember,
		roleCreatorNotMember,
		roleAssigneeNotMember,
		roleOutsider,
	}
	actions := []authzAction{
		actionRead,
		actionComment,
		actionEdit,
		actionDelete,
		actionUpload,
		actionModerateComments,
		actionModerateAttachments,
	}

	// 7 roles * 7 actions * 3 states = 147 test cells
	for _, state := range states {
		for _, role := range roles {
			for _, action := range actions {
				testName := fmt.Sprintf("%s/%s/%s", state, role, action)
				t.Run(testName, func(t *testing.T) {
					// Setup isolated task fixture for each cell
					var pID uuid.UUID
					if state == stateProjectSoftDeleted {
						pID = deletedProject.ID
					} else {
						pID = normalProject.ID
					}

					assigneeID := assigneeNotMemberUser.ID
					task := createTestTask(ctx, t, creatorNotMemberUser.ID, &pID, &assigneeID, "Task for "+testName)

					if state == stateTaskSoftDeleted {
						_, delErr := testPool.Exec(ctx, "UPDATE tasks SET deleted_at = NOW() WHERE id = $1", task.ID)
						require.NoError(t, delErr)
					}

					// Create sub-resources authored by owner (so cross-user Moderate actions can be tested)
					comment := createTestComment(ctx, t, task.ID, ownerUser.ID, "Owner comment for "+testName)
					attachment := createTestAttachment(ctx, t, task.ID, ownerUser.ID, "doc.txt")

					caller := userMap[role]
					exp := getExpectedOutcome(state, role, action)

					var actionErr error
					switch action {
					case actionRead:
						_, actionErr = taskService.GetTaskByID(ctx, caller.ID, task.ID)

					case actionComment:
						_, actionErr = commentService.CreateComment(ctx, caller.ID, task.ID, service.CreateCommentInput{
							Content: "Test comment content",
						})

					case actionEdit:
						_, actionErr = taskService.UpdateTask(ctx, caller.ID, task.ID, service.UpdateTaskInput{
							Title:   "Updated Title",
							Version: task.Version,
						})

					case actionDelete:
						actionErr = taskService.DeleteTask(ctx, caller.ID, task.ID)

					case actionUpload:
						txtContent := []byte("plain text attachment content for upload")
						_, actionErr = attachmentService.UploadAttachment(
							ctx,
							caller.ID,
							task.ID,
							"upload.txt",
							int64(len(txtContent)),
							"text/plain",
							bytes.NewReader(txtContent),
						)

					case actionModerateComments:
						actionErr = commentService.DeleteComment(ctx, caller.ID, task.ID, comment.ID)

					case actionModerateAttachments:
						actionErr = attachmentService.DeleteAttachment(ctx, caller.ID, task.ID, attachment.ID)
					}

					switch exp {
					case outcomeAllow:
						assert.NoError(t, actionErr, "expected ALLOW for %s", testName)
					case outcomeDeny403:
						assert.ErrorIs(t, actionErr, service.ErrInsufficientProjectRole, "expected 403 ErrInsufficientProjectRole for %s", testName)
					case outcomeDeny404:
						assert.ErrorIs(t, actionErr, service.ErrTaskNotFound, "expected 404 ErrTaskNotFound for %s", testName)
					}
				})
			}
		}
	}
}

func TestIntegration_AuthzSelfAuthored(t *testing.T) {
	ctx := context.Background()

	commentService := service.NewCommentService(testStore)
	attachmentService := service.NewAttachmentService(testStore, &inMemoryStorage{}, 10)
	projectService := service.NewProjectService(testStore, cache.NewNoopCache())

	owner := createTestUser(ctx, t, "self_owner")
	editor := createTestUser(ctx, t, "self_editor")
	viewer := createTestUser(ctx, t, "self_viewer")
	leaver := createTestUser(ctx, t, "self_leaver")

	proj := createTestProject(ctx, t, owner.ID, "Self Authored Project")
	addTestMember(ctx, t, proj.ID, editor.ID, "editor")
	addTestMember(ctx, t, proj.ID, viewer.ID, "viewer")
	addTestMember(ctx, t, proj.ID, leaver.ID, "editor")

	task := createTestTask(ctx, t, owner.ID, &proj.ID, nil, "Self Authored Task")

	// 1. Editor deletes their OWN comment -> ALLOW
	t.Run("EditorDeletesOwnComment", func(t *testing.T) {
		comment := createTestComment(ctx, t, task.ID, editor.ID, "Editor's own comment")
		err := commentService.DeleteComment(ctx, editor.ID, task.ID, comment.ID)
		assert.NoError(t, err)
	})

	// 2. Viewer deletes their OWN comment -> ALLOW
	t.Run("ViewerDeletesOwnComment", func(t *testing.T) {
		comment := createTestComment(ctx, t, task.ID, viewer.ID, "Viewer's own comment")
		err := commentService.DeleteComment(ctx, viewer.ID, task.ID, comment.ID)
		assert.NoError(t, err)
	})

	// 3. Editor deletes their OWN attachment -> ALLOW
	t.Run("EditorDeletesOwnAttachment", func(t *testing.T) {
		att := createTestAttachment(ctx, t, task.ID, editor.ID, "editor_file.txt")
		err := attachmentService.DeleteAttachment(ctx, editor.ID, task.ID, att.ID)
		assert.NoError(t, err)
	})

	// 4. Leaver creates comment and attachment, then leaves project -> loses all access (404)
	t.Run("RemovedMemberCannotInteractWithOwnPastContent", func(t *testing.T) {
		leaverComment := createTestComment(ctx, t, task.ID, leaver.ID, "Leaver's comment before departure")
		leaverAtt := createTestAttachment(ctx, t, task.ID, leaver.ID, "leaver_file.txt")

		// Leaver leaves project
		err := projectService.RemoveMember(ctx, leaver.ID, proj.ID, leaver.ID)
		require.NoError(t, err)

		// Cannot delete own comment -> 404
		cErr := commentService.DeleteComment(ctx, leaver.ID, task.ID, leaverComment.ID)
		assert.ErrorIs(t, cErr, service.ErrTaskNotFound)

		// Cannot delete own attachment -> 404
		aErr := attachmentService.DeleteAttachment(ctx, leaver.ID, task.ID, leaverAtt.ID)
		assert.ErrorIs(t, aErr, service.ErrTaskNotFound)
	})
}

func TestIntegration_AuthzPersonalTasks(t *testing.T) {
	ctx := context.Background()

	taskService := service.NewTaskService(testStore)
	commentService := service.NewCommentService(testStore)
	attachmentService := service.NewAttachmentService(testStore, &inMemoryStorage{}, 10)

	creator := createTestUser(ctx, t, "personal_creator")
	assignee := createTestUser(ctx, t, "personal_assignee")
	outsider := createTestUser(ctx, t, "personal_outsider")

	assigneeID := assignee.ID
	task := createTestTask(ctx, t, creator.ID, nil, &assigneeID, "Personal Task")

	comment := createTestComment(ctx, t, task.ID, creator.ID, "Creator comment on personal task")
	attachment := createTestAttachment(ctx, t, task.ID, creator.ID, "creator_file.txt")

	// Creator has full administrative control
	t.Run("CreatorHasFullControl", func(t *testing.T) {
		_, err := taskService.GetTaskByID(ctx, creator.ID, task.ID)
		assert.NoError(t, err)

		_, err = commentService.CreateComment(ctx, creator.ID, task.ID, service.CreateCommentInput{Content: "c"})
		assert.NoError(t, err)

		_, err = taskService.UpdateTask(ctx, creator.ID, task.ID, service.UpdateTaskInput{Title: "T2", Version: 1})
		assert.NoError(t, err)

		txt := []byte("upload")
		_, err = attachmentService.UploadAttachment(ctx, creator.ID, task.ID, "f.txt", int64(len(txt)), "text/plain", bytes.NewReader(txt))
		assert.NoError(t, err)

		err = commentService.DeleteComment(ctx, creator.ID, task.ID, comment.ID)
		assert.NoError(t, err)

		err = attachmentService.DeleteAttachment(ctx, creator.ID, task.ID, attachment.ID)
		assert.NoError(t, err)
	})

	// Assignee has collaborative rights (read, comment, upload) but cannot edit, delete, or moderate
	t.Run("AssigneeOperationalRights", func(t *testing.T) {
		pTask := createTestTask(ctx, t, creator.ID, nil, &assigneeID, "Personal Task 2")
		pComment := createTestComment(ctx, t, pTask.ID, creator.ID, "Comment")
		pAtt := createTestAttachment(ctx, t, pTask.ID, creator.ID, "file.txt")

		// Read -> ALLOW
		_, err := taskService.GetTaskByID(ctx, assignee.ID, pTask.ID)
		assert.NoError(t, err)

		// Comment -> ALLOW
		_, err = commentService.CreateComment(ctx, assignee.ID, pTask.ID, service.CreateCommentInput{Content: "Assignee comment"})
		assert.NoError(t, err)

		// Upload -> ALLOW
		txt := []byte("assignee upload")
		_, err = attachmentService.UploadAttachment(ctx, assignee.ID, pTask.ID, "f.txt", int64(len(txt)), "text/plain", bytes.NewReader(txt))
		assert.NoError(t, err)

		// Edit -> 403
		_, err = taskService.UpdateTask(ctx, assignee.ID, pTask.ID, service.UpdateTaskInput{Title: "Hacked", Version: 1})
		assert.ErrorIs(t, err, service.ErrInsufficientProjectRole)

		// Delete -> 403
		err = taskService.DeleteTask(ctx, assignee.ID, pTask.ID)
		assert.ErrorIs(t, err, service.ErrInsufficientProjectRole)

		// Moderate comments -> 403
		err = commentService.DeleteComment(ctx, assignee.ID, pTask.ID, pComment.ID)
		assert.ErrorIs(t, err, service.ErrInsufficientProjectRole)

		// Moderate attachments -> 403
		err = attachmentService.DeleteAttachment(ctx, assignee.ID, pTask.ID, pAtt.ID)
		assert.ErrorIs(t, err, service.ErrInsufficientProjectRole)
	})

	// Outsider has zero access (404)
	t.Run("OutsiderHasZeroAccess", func(t *testing.T) {
		pTask := createTestTask(ctx, t, creator.ID, nil, &assigneeID, "Personal Task 3")
		pComment := createTestComment(ctx, t, pTask.ID, creator.ID, "Comment")
		pAtt := createTestAttachment(ctx, t, pTask.ID, creator.ID, "file.txt")

		_, err := taskService.GetTaskByID(ctx, outsider.ID, pTask.ID)
		assert.ErrorIs(t, err, service.ErrTaskNotFound)

		_, err = commentService.CreateComment(ctx, outsider.ID, pTask.ID, service.CreateCommentInput{Content: "c"})
		assert.ErrorIs(t, err, service.ErrTaskNotFound)

		_, err = taskService.UpdateTask(ctx, outsider.ID, pTask.ID, service.UpdateTaskInput{Title: "Hacked", Version: 1})
		assert.ErrorIs(t, err, service.ErrTaskNotFound)

		err = taskService.DeleteTask(ctx, outsider.ID, pTask.ID)
		assert.ErrorIs(t, err, service.ErrTaskNotFound)

		txt := []byte("outsider upload")
		_, err = attachmentService.UploadAttachment(ctx, outsider.ID, pTask.ID, "f.txt", int64(len(txt)), "text/plain", bytes.NewReader(txt))
		assert.ErrorIs(t, err, service.ErrTaskNotFound)

		err = commentService.DeleteComment(ctx, outsider.ID, pTask.ID, pComment.ID)
		assert.ErrorIs(t, err, service.ErrTaskNotFound)

		err = attachmentService.DeleteAttachment(ctx, outsider.ID, pTask.ID, pAtt.ID)
		assert.ErrorIs(t, err, service.ErrTaskNotFound)
	})
}
