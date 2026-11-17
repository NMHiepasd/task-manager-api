//go:build integration

package repository_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"task-manager-api/internal/repository"
)

func TestIntegration_Webhook_ProjectANotReceivesProjectB(t *testing.T) {
	ctx := context.Background()

	// 1. Create User
	email := fmt.Sprintf("user_wh_scope_%d@example.com", time.Now().UnixNano())
	user, err := testStore.CreateUser(ctx, repository.CreateUserParams{
		Email:        email,
		PasswordHash: "hashed_secret_123",
		FullName:     "Webhook Scope User",
		Role:         "user",
	})
	require.NoError(t, err)

	// 2. Create Project A and Project B owned by the same user
	projA, err := testStore.CreateProject(ctx, repository.CreateProjectParams{
		OwnerID:     user.ID,
		Name:        "Project Alpha",
		Description: "Alpha Description",
		Color:       "#3b82f6",
	})
	require.NoError(t, err)

	projB, err := testStore.CreateProject(ctx, repository.CreateProjectParams{
		OwnerID:     user.ID,
		Name:        "Project Beta",
		Description: "Beta Description",
		Color:       "#ef4444",
	})
	require.NoError(t, err)

	// 3. Register webhook specifically bound to Project A
	whA, err := testStore.CreateWebhook(ctx, repository.CreateWebhookParams{
		UserID:    user.ID,
		ProjectID: pgtype.UUID{Bytes: projA.ID, Valid: true},
		Url:       "https://example.com/webhook-proj-a",
		Secret:    "whsec_proj_a_secret_32bytes_min!",
		Events:    []string{"task:created"},
		IsActive:  true,
	})
	require.NoError(t, err)

	// 4. Dispatch event occurring in Project B triggered by the user
	matched, err := testStore.ListActiveWebhooksForEvent(ctx, repository.ListActiveWebhooksForEventParams{
		UserID:    user.ID,
		ProjectID: pgtype.UUID{Bytes: projB.ID, Valid: true},
		EventType: "task:created",
	})
	require.NoError(t, err)

	// Webhook bound to Project A must NEVER receive events occurring in Project B
	for _, wh := range matched {
		assert.NotEqual(t, whA.ID, wh.ID, "Webhook bound to Project A must not receive events from Project B")
	}
}

func TestIntegration_Webhook_RemovedEditorNotReceivesEvent(t *testing.T) {
	ctx := context.Background()

	// 1. Create Owner and Editor users
	ownerEmail := fmt.Sprintf("owner_%d@example.com", time.Now().UnixNano())
	owner, err := testStore.CreateUser(ctx, repository.CreateUserParams{
		Email:        ownerEmail,
		PasswordHash: "hashed_secret_123",
		FullName:     "Project Owner",
		Role:         "user",
	})
	require.NoError(t, err)

	editorEmail := fmt.Sprintf("editor_%d@example.com", time.Now().UnixNano())
	editor, err := testStore.CreateUser(ctx, repository.CreateUserParams{
		Email:        editorEmail,
		PasswordHash: "hashed_secret_123",
		FullName:     "Project Editor",
		Role:         "user",
	})
	require.NoError(t, err)

	// 2. Create Project
	proj, err := testStore.CreateProject(ctx, repository.CreateProjectParams{
		OwnerID:     owner.ID,
		Name:        "Shared Project",
		Description: "Shared collaboration project",
		Color:       "#10b981",
	})
	require.NoError(t, err)

	// 3. Add Editor as project member with 'editor' role
	_, err = testStore.AddProjectMember(ctx, repository.AddProjectMemberParams{
		ProjectID: proj.ID,
		UserID:    editor.ID,
		Role:      "editor",
	})
	require.NoError(t, err)

	// 4. Editor creates a webhook on this project
	whEditor, err := testStore.CreateWebhook(ctx, repository.CreateWebhookParams{
		UserID:    editor.ID,
		ProjectID: pgtype.UUID{Bytes: proj.ID, Valid: true},
		Url:       "https://example.com/editor-webhook",
		Secret:    "whsec_editor_secret_32bytes_min!",
		Events:    []string{"task:created"},
		IsActive:  true,
	})
	require.NoError(t, err)

	// 5. Remove Editor from the project
	err = testStore.DeleteProjectMember(ctx, repository.DeleteProjectMemberParams{
		ProjectID: proj.ID,
		UserID:    editor.ID,
	})
	require.NoError(t, err)

	// 6. Owner triggers an event in the project
	matched, err := testStore.ListActiveWebhooksForEvent(ctx, repository.ListActiveWebhooksForEventParams{
		UserID:    owner.ID,
		ProjectID: pgtype.UUID{Bytes: proj.ID, Valid: true},
		EventType: "task:created",
	})
	require.NoError(t, err)

	// Former editor's webhook must NOT be selected for dispatching events in the project
	for _, wh := range matched {
		assert.NotEqual(t, whEditor.ID, wh.ID, "Removed editor's webhook must not receive project events")
	}
}

func TestIntegration_Webhook_PersonalWebhookReceivesOwnerEvents(t *testing.T) {
	ctx := context.Background()

	// 1. Create User
	email := fmt.Sprintf("personal_wh_%d@example.com", time.Now().UnixNano())
	user, err := testStore.CreateUser(ctx, repository.CreateUserParams{
		Email:        email,
		PasswordHash: "hashed_secret_123",
		FullName:     "Personal Webhook User",
		Role:         "user",
	})
	require.NoError(t, err)

	// 2. Create personal webhook (project_id IS NULL)
	whPersonal, err := testStore.CreateWebhook(ctx, repository.CreateWebhookParams{
		UserID:    user.ID,
		ProjectID: pgtype.UUID{Valid: false},
		Url:       "https://example.com/personal-webhook",
		Secret:    "whsec_personal_secret_32bytes_min",
		Events:    []string{"task:created"},
		IsActive:  true,
	})
	require.NoError(t, err)

	// 3. User triggers an event without a project
	matched, err := testStore.ListActiveWebhooksForEvent(ctx, repository.ListActiveWebhooksForEventParams{
		UserID:    user.ID,
		ProjectID: pgtype.UUID{Valid: false},
		EventType: "task:created",
	})
	require.NoError(t, err)

	// Personal webhook must be selected
	found := false
	for _, wh := range matched {
		if wh.ID == whPersonal.ID {
			found = true
			break
		}
	}
	assert.True(t, found, "Personal webhook should receive events triggered by its owner")
}
