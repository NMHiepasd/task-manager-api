package service

import (
	"context"
	"encoding/json"
	"task-manager-api/internal/domain"
	"task-manager-api/internal/repository"
	"task-manager-api/internal/worker"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockWebhookDistributor struct {
	sent []worker.SendWebhookPayload
}

func (m *mockWebhookDistributor) DistributeCleanupExpiredTokens(context.Context, ...asynq.Option) error {
	return nil
}

func (m *mockWebhookDistributor) DistributeCleanupExpiredIdempotencyKeys(context.Context, ...asynq.Option) error {
	return nil
}

func (m *mockWebhookDistributor) DistributeScanDueTasks(context.Context, ...asynq.Option) error {
	return nil
}

func (m *mockWebhookDistributor) DistributeDeadlineReminder(context.Context, worker.DeadlineReminderPayload, ...asynq.Option) error {
	return nil
}

func (m *mockWebhookDistributor) DistributeSendWebhook(_ context.Context, payload worker.SendWebhookPayload, _ ...asynq.Option) error {
	m.sent = append(m.sent, payload)
	return nil
}

func (m *mockWebhookDistributor) DistributeDeleteStorageObject(context.Context, worker.DeleteStorageObjectPayload, ...asynq.Option) error {
	return nil
}

func (m *mockWebhookDistributor) Close() error { return nil }

type mockWebhookQuerier struct {
	mockQuerier
	createWebhookFn              func(ctx context.Context, arg repository.CreateWebhookParams) (repository.Webhook, error)
	getWebhookByIDFn             func(ctx context.Context, id uuid.UUID) (repository.Webhook, error)
	listWebhooksByUserIDFn       func(ctx context.Context, userID uuid.UUID) ([]repository.Webhook, error)
	listWebhooksByProjectIDFn    func(ctx context.Context, projectID pgtype.UUID) ([]repository.Webhook, error)
	updateWebhookFn              func(ctx context.Context, arg repository.UpdateWebhookParams) (repository.Webhook, error)
	deleteWebhookFn              func(ctx context.Context, id uuid.UUID) error
	listActiveWebhooksForEventFn func(ctx context.Context, arg repository.ListActiveWebhooksForEventParams) ([]repository.Webhook, error)
	listWebhookDeliveriesFn      func(ctx context.Context, arg repository.ListWebhookDeliveriesParams) ([]repository.WebhookDelivery, error)
	getProjectByIDFn             func(ctx context.Context, id uuid.UUID) (repository.Project, error)
	getProjectMemberFn           func(ctx context.Context, arg repository.GetProjectMemberParams) (repository.ProjectMember, error)
}

func (m *mockWebhookQuerier) CreateWebhook(ctx context.Context, arg repository.CreateWebhookParams) (repository.Webhook, error) {
	if m.createWebhookFn != nil {
		return m.createWebhookFn(ctx, arg)
	}
	return repository.Webhook{}, nil
}

func (m *mockWebhookQuerier) GetWebhookByID(ctx context.Context, id uuid.UUID) (repository.Webhook, error) {
	if m.getWebhookByIDFn != nil {
		return m.getWebhookByIDFn(ctx, id)
	}
	return repository.Webhook{}, pgx.ErrNoRows
}

func (m *mockWebhookQuerier) ListWebhooksByUserID(ctx context.Context, userID uuid.UUID) ([]repository.Webhook, error) {
	if m.listWebhooksByUserIDFn != nil {
		return m.listWebhooksByUserIDFn(ctx, userID)
	}
	return nil, nil
}

func (m *mockWebhookQuerier) ListWebhooksByProjectID(ctx context.Context, projectID pgtype.UUID) ([]repository.Webhook, error) {
	if m.listWebhooksByProjectIDFn != nil {
		return m.listWebhooksByProjectIDFn(ctx, projectID)
	}
	return nil, nil
}

func (m *mockWebhookQuerier) UpdateWebhook(ctx context.Context, arg repository.UpdateWebhookParams) (repository.Webhook, error) {
	if m.updateWebhookFn != nil {
		return m.updateWebhookFn(ctx, arg)
	}
	return repository.Webhook{}, nil
}

func (m *mockWebhookQuerier) DeleteWebhook(ctx context.Context, id uuid.UUID) error {
	if m.deleteWebhookFn != nil {
		return m.deleteWebhookFn(ctx, id)
	}
	return nil
}

func (m *mockWebhookQuerier) ListActiveWebhooksForEvent(ctx context.Context, arg repository.ListActiveWebhooksForEventParams) ([]repository.Webhook, error) {
	if m.listActiveWebhooksForEventFn != nil {
		return m.listActiveWebhooksForEventFn(ctx, arg)
	}
	return nil, nil
}

func (m *mockWebhookQuerier) ListWebhookDeliveries(ctx context.Context, arg repository.ListWebhookDeliveriesParams) ([]repository.WebhookDelivery, error) {
	if m.listWebhookDeliveriesFn != nil {
		return m.listWebhookDeliveriesFn(ctx, arg)
	}
	return nil, nil
}

func (m *mockWebhookQuerier) GetProjectByID(ctx context.Context, id uuid.UUID) (repository.Project, error) {
	if m.getProjectByIDFn != nil {
		return m.getProjectByIDFn(ctx, id)
	}
	return repository.Project{}, pgx.ErrNoRows
}

func (m *mockWebhookQuerier) GetProjectMember(ctx context.Context, arg repository.GetProjectMemberParams) (repository.ProjectMember, error) {
	if m.getProjectMemberFn != nil {
		return m.getProjectMemberFn(ctx, arg)
	}
	return repository.ProjectMember{}, pgx.ErrNoRows
}

func TestWebhookService_CreateWebhook(t *testing.T) {
	userID := uuid.New()

	t.Run("Success_AutoGenerateSecret", func(t *testing.T) {
		dist := &mockWebhookDistributor{}
		mockQ := &mockWebhookQuerier{
			createWebhookFn: func(_ context.Context, arg repository.CreateWebhookParams) (repository.Webhook, error) {
				return repository.Webhook{
					ID:        uuid.New(),
					UserID:    arg.UserID,
					Url:       arg.Url,
					Secret:    arg.Secret,
					Events:    arg.Events,
					IsActive:  arg.IsActive,
					CreatedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
					UpdatedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
				}, nil
			},
		}

		svc := NewWebhookService(mockQ, dist, true)
		res, err := svc.CreateWebhook(context.Background(), userID, CreateWebhookRequest{
			URL:    "http://localhost:8080/hook",
			Events: []string{"task.created", "task.deleted"},
		})
		require.NoError(t, err)
		assert.NotEmpty(t, res.ID)
		assert.NotEmpty(t, res.Secret)
		assert.GreaterOrEqual(t, len(res.Secret), 32)
		assert.True(t, res.IsActive)
	})

	t.Run("Success_CustomSecretValid", func(t *testing.T) {
		dist := &mockWebhookDistributor{}
		mockQ := &mockWebhookQuerier{
			createWebhookFn: func(_ context.Context, arg repository.CreateWebhookParams) (repository.Webhook, error) {
				return repository.Webhook{
					ID:        uuid.New(),
					UserID:    arg.UserID,
					Url:       arg.Url,
					Secret:    arg.Secret,
					Events:    arg.Events,
					IsActive:  arg.IsActive,
					CreatedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
					UpdatedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
				}, nil
			},
		}

		//nolint:gosec // G101: Test fixture dummy secret
		customSecret := "whsec_custom_secret_key_32_characters_minimum!"
		svc := NewWebhookService(mockQ, dist, true)
		res, err := svc.CreateWebhook(context.Background(), userID, CreateWebhookRequest{
			URL:    "http://localhost:8080/hook",
			Secret: customSecret,
			Events: []string{"task.created"},
		})
		require.NoError(t, err)
		assert.Equal(t, customSecret, res.Secret)
	})

	t.Run("Fail_CustomSecretTooShort", func(t *testing.T) {
		dist := &mockWebhookDistributor{}
		mockQ := &mockWebhookQuerier{}

		svc := NewWebhookService(mockQ, dist, true)
		_, err := svc.CreateWebhook(context.Background(), userID, CreateWebhookRequest{
			URL:    "http://localhost:8080/hook",
			Secret: "short_secret_under_32_chars",
			Events: []string{"task.created"},
		})
		assert.ErrorIs(t, err, domain.ErrInvalidWebhookSecret)
	})

	t.Run("Fail_SSRFBlocked", func(t *testing.T) {
		dist := &mockWebhookDistributor{}
		mockQ := &mockWebhookQuerier{}

		svc := NewWebhookService(mockQ, dist, false) // SSRF protection enabled
		_, err := svc.CreateWebhook(context.Background(), userID, CreateWebhookRequest{
			URL:    "http://localhost:8080/hook",
			Events: []string{"task.created"},
		})
		assert.ErrorIs(t, err, domain.ErrWebhookSSRF)
	})

	t.Run("Fail_InvalidEvents", func(t *testing.T) {
		dist := &mockWebhookDistributor{}
		mockQ := &mockWebhookQuerier{}

		svc := NewWebhookService(mockQ, dist, true)
		_, err := svc.CreateWebhook(context.Background(), userID, CreateWebhookRequest{
			URL:    "https://example.com/hook",
			Events: []string{"unknown.event"},
		})
		assert.ErrorIs(t, err, domain.ErrInvalidWebhookEvents)
	})
}

func TestWebhookService_GetAndListWebhooks(t *testing.T) {
	userID := uuid.New()
	webhookID := uuid.New()

	t.Run("GetWebhook_Success", func(t *testing.T) {
		mockQ := &mockWebhookQuerier{
			getWebhookByIDFn: func(_ context.Context, id uuid.UUID) (repository.Webhook, error) {
				return repository.Webhook{
					ID:        id,
					UserID:    userID,
					Url:       "https://example.com/hook",
					Secret:    "whsec_test",
					Events:    []string{"task.created"},
					IsActive:  true,
					CreatedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
				}, nil
			},
		}

		svc := NewWebhookService(mockQ, nil, false)
		res, err := svc.GetWebhook(context.Background(), userID, webhookID)
		require.NoError(t, err)
		assert.Equal(t, webhookID, res.ID)
		assert.Empty(t, res.Secret, "secret must not be revealed in GetWebhook response")
	})

	t.Run("GetWebhook_Forbidden_AntiIDOR", func(t *testing.T) {
		attackerID := uuid.New()
		mockQ := &mockWebhookQuerier{
			getWebhookByIDFn: func(_ context.Context, id uuid.UUID) (repository.Webhook, error) {
				return repository.Webhook{
					ID:     id,
					UserID: userID,
				}, nil
			},
		}

		svc := NewWebhookService(mockQ, nil, false)
		_, err := svc.GetWebhook(context.Background(), attackerID, webhookID)
		assert.ErrorIs(t, err, domain.ErrWebhookNotFound)
	})

	t.Run("ListWebhooks_UserLevel", func(t *testing.T) {
		mockQ := &mockWebhookQuerier{
			listWebhooksByUserIDFn: func(_ context.Context, uid uuid.UUID) ([]repository.Webhook, error) {
				return []repository.Webhook{
					{ID: uuid.New(), UserID: uid, Url: "https://example.com/h1", Events: []string{"*"}},
					{ID: uuid.New(), UserID: uid, Url: "https://example.com/h2", Events: []string{"task.created"}},
				}, nil
			},
		}

		svc := NewWebhookService(mockQ, nil, false)
		list, err := svc.ListWebhooks(context.Background(), userID, nil)
		require.NoError(t, err)
		assert.Len(t, list, 2)
		assert.Empty(t, list[0].Secret, "secret must not be revealed in ListWebhooks")
		assert.Empty(t, list[1].Secret, "secret must not be revealed in ListWebhooks")
	})
}

func TestWebhookService_UpdateAndDelete(t *testing.T) {
	userID := uuid.New()
	webhookID := uuid.New()

	t.Run("UpdateWebhook_Success", func(t *testing.T) {
		mockQ := &mockWebhookQuerier{
			getWebhookByIDFn: func(_ context.Context, id uuid.UUID) (repository.Webhook, error) {
				return repository.Webhook{ID: id, UserID: userID, Url: "https://example.com/old"}, nil
			},
			updateWebhookFn: func(_ context.Context, arg repository.UpdateWebhookParams) (repository.Webhook, error) {
				return repository.Webhook{
					ID:        arg.ID,
					UserID:    userID,
					Url:       arg.Url.String,
					Events:    arg.Events,
					IsActive:  arg.IsActive.Bool,
					UpdatedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
				}, nil
			},
		}

		svc := NewWebhookService(mockQ, nil, true)
		newURL := "https://example.com/new"
		isActive := false
		res, err := svc.UpdateWebhook(context.Background(), userID, webhookID, UpdateWebhookRequest{
			URL:      &newURL,
			IsActive: &isActive,
		})
		require.NoError(t, err)
		assert.Equal(t, newURL, res.URL)
		assert.False(t, res.IsActive)
		assert.Empty(t, res.Secret, "secret must not be revealed in UpdateWebhook response")
	})

	t.Run("DeleteWebhook_Success", func(t *testing.T) {
		deleted := false
		mockQ := &mockWebhookQuerier{
			getWebhookByIDFn: func(_ context.Context, id uuid.UUID) (repository.Webhook, error) {
				return repository.Webhook{ID: id, UserID: userID}, nil
			},
			deleteWebhookFn: func(_ context.Context, _ uuid.UUID) error {
				deleted = true
				return nil
			},
		}

		svc := NewWebhookService(mockQ, nil, true)
		err := svc.DeleteWebhook(context.Background(), userID, webhookID)
		assert.NoError(t, err)
		assert.True(t, deleted)
	})
}

func TestWebhookService_TriggerTestPingAndDispatch(t *testing.T) {
	userID := uuid.New()
	webhookID := uuid.New()

	t.Run("TriggerTestPing_EnqueuesTask", func(t *testing.T) {
		dist := &mockWebhookDistributor{}
		mockQ := &mockWebhookQuerier{
			getWebhookByIDFn: func(_ context.Context, id uuid.UUID) (repository.Webhook, error) {
				return repository.Webhook{
					ID:     id,
					UserID: userID,
					Url:    "https://example.com/hook",
					Secret: "whsec_test",
				}, nil
			},
		}

		svc := NewWebhookService(mockQ, dist, false)
		err := svc.TriggerTestPing(context.Background(), userID, webhookID)
		require.NoError(t, err)
		require.Len(t, dist.sent, 1)
		assert.Equal(t, "ping", dist.sent[0].EventType)
		assert.Equal(t, webhookID, dist.sent[0].WebhookID)
		assert.NotEqual(t, uuid.Nil, dist.sent[0].DeliveryID)

		var parsed map[string]any
		err = json.Unmarshal(dist.sent[0].Payload, &parsed)
		require.NoError(t, err)
		assert.Equal(t, dist.sent[0].DeliveryID.String(), parsed["id"])
	})

	t.Run("DispatchEvent_EnqueuesActiveWebhooks", func(t *testing.T) {
		dist := &mockWebhookDistributor{}
		wh1 := repository.Webhook{ID: uuid.New(), UserID: userID, Url: "https://hook1.com", Secret: "s1"}
		wh2 := repository.Webhook{ID: uuid.New(), UserID: userID, Url: "https://hook2.com", Secret: "s2"}

		mockQ := &mockWebhookQuerier{
			listActiveWebhooksForEventFn: func(_ context.Context, _ repository.ListActiveWebhooksForEventParams) ([]repository.Webhook, error) {
				return []repository.Webhook{wh1, wh2}, nil
			},
		}

		svc := NewWebhookService(mockQ, dist, false)
		err := svc.DispatchEvent(context.Background(), userID, nil, "task.created", map[string]any{"task_id": "123"})
		require.NoError(t, err)
		require.Len(t, dist.sent, 2)
		assert.Equal(t, "task.created", dist.sent[0].EventType)
		assert.Equal(t, "task.created", dist.sent[1].EventType)
		assert.NotEqual(t, uuid.Nil, dist.sent[0].DeliveryID)
		assert.NotEqual(t, uuid.Nil, dist.sent[1].DeliveryID)
		assert.NotEqual(t, dist.sent[0].DeliveryID, dist.sent[1].DeliveryID, "each webhook target gets a unique delivery_id")

		var parsed1, parsed2 map[string]any
		require.NoError(t, json.Unmarshal(dist.sent[0].Payload, &parsed1))
		require.NoError(t, json.Unmarshal(dist.sent[1].Payload, &parsed2))
		assert.Equal(t, dist.sent[0].DeliveryID.String(), parsed1["id"])
		assert.Equal(t, dist.sent[1].DeliveryID.String(), parsed2["id"])
	})
}

func TestWebhookService_ADR0019_AuthorizationMatrix(t *testing.T) {
	ctx := context.Background()
	projectID := uuid.New()
	ownerID := uuid.New()
	creatorID := uuid.New()
	peerEditorID := uuid.New()
	viewerID := uuid.New()
	formerMemberID := uuid.New()
	demotedCreatorID := uuid.New()

	webhookID := uuid.New()
	demotedWebhookID := uuid.New()

	//nolint:gosec // G101: Test fixture dummy secret
	targetWebhook := repository.Webhook{
		ID:        webhookID,
		UserID:    creatorID,
		ProjectID: pgtype.UUID{Bytes: projectID, Valid: true},
		Url:       "https://example.com/target",
		Secret:    "whsec_target_secret",
		Events:    []string{"task.created"},
		IsActive:  true,
		CreatedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
	}

	//nolint:gosec // G101: Test fixture dummy secret
	demotedWebhook := repository.Webhook{
		ID:        demotedWebhookID,
		UserID:    demotedCreatorID,
		ProjectID: pgtype.UUID{Bytes: projectID, Valid: true},
		Url:       "https://example.com/demoted",
		Secret:    "whsec_demoted_secret",
		Events:    []string{"task.created"},
		IsActive:  false, // Deactivated on demotion
		CreatedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
	}

	project := repository.Project{
		ID:      projectID,
		OwnerID: ownerID,
	}

	mockQ := &mockWebhookQuerier{
		getWebhookByIDFn: func(_ context.Context, id uuid.UUID) (repository.Webhook, error) {
			if id == webhookID {
				return targetWebhook, nil
			}
			if id == demotedWebhookID {
				return demotedWebhook, nil
			}
			return repository.Webhook{}, pgx.ErrNoRows
		},
		getProjectByIDFn: func(_ context.Context, id uuid.UUID) (repository.Project, error) {
			if id == projectID {
				return project, nil
			}
			return repository.Project{}, pgx.ErrNoRows
		},
		getProjectMemberFn: func(_ context.Context, arg repository.GetProjectMemberParams) (repository.ProjectMember, error) {
			if arg.ProjectID != projectID {
				return repository.ProjectMember{}, pgx.ErrNoRows
			}
			switch arg.UserID {
			case creatorID:
				return repository.ProjectMember{ProjectID: projectID, UserID: creatorID, Role: "editor"}, nil
			case peerEditorID:
				return repository.ProjectMember{ProjectID: projectID, UserID: peerEditorID, Role: "editor"}, nil
			case viewerID:
				return repository.ProjectMember{ProjectID: projectID, UserID: viewerID, Role: "viewer"}, nil
			case demotedCreatorID:
				return repository.ProjectMember{ProjectID: projectID, UserID: demotedCreatorID, Role: "viewer"}, nil
			default:
				return repository.ProjectMember{}, pgx.ErrNoRows
			}
		},
		updateWebhookFn: func(_ context.Context, arg repository.UpdateWebhookParams) (repository.Webhook, error) {
			return repository.Webhook{
				ID:        arg.ID,
				UserID:    creatorID,
				ProjectID: pgtype.UUID{Bytes: projectID, Valid: true},
				Url:       "https://example.com/updated",
				IsActive:  true,
			}, nil
		},
		deleteWebhookFn: func(_ context.Context, _ uuid.UUID) error {
			return nil
		},
		listWebhookDeliveriesFn: func(_ context.Context, _ repository.ListWebhookDeliveriesParams) ([]repository.WebhookDelivery, error) {
			return []repository.WebhookDelivery{
				{ID: uuid.New(), WebhookID: webhookID, Status: "success"},
			}, nil
		},
		listWebhooksByProjectIDFn: func(_ context.Context, _ pgtype.UUID) ([]repository.Webhook, error) {
			return []repository.Webhook{targetWebhook, demotedWebhook}, nil
		},
	}

	dist := &mockWebhookDistributor{}
	svc := NewWebhookService(mockQ, dist, false)

	newURL := "https://example.com/updated"
	updateReq := UpdateWebhookRequest{URL: &newURL}

	// 1. Project Owner: ALLOW for all 5 operations
	t.Run("ProjectOwner_AllowedAllOperations", func(t *testing.T) {
		res, err := svc.GetWebhook(ctx, ownerID, webhookID)
		require.NoError(t, err)
		assert.Equal(t, webhookID, res.ID)

		upRes, err := svc.UpdateWebhook(ctx, ownerID, webhookID, updateReq)
		require.NoError(t, err)
		assert.Equal(t, "https://example.com/updated", upRes.URL)

		err = svc.TriggerTestPing(ctx, ownerID, webhookID)
		require.NoError(t, err)

		deliveries, err := svc.ListDeliveries(ctx, ownerID, webhookID, 10, 0)
		require.NoError(t, err)
		assert.Len(t, deliveries, 1)

		err = svc.DeleteWebhook(ctx, ownerID, webhookID)
		require.NoError(t, err)
	})

	// 2. Creator (Active Editor): ALLOW for all 5 operations
	t.Run("Creator_ActiveEditor_AllowedAllOperations", func(t *testing.T) {
		res, err := svc.GetWebhook(ctx, creatorID, webhookID)
		require.NoError(t, err)
		assert.Equal(t, webhookID, res.ID)

		upRes, err := svc.UpdateWebhook(ctx, creatorID, webhookID, updateReq)
		require.NoError(t, err)
		assert.Equal(t, "https://example.com/updated", upRes.URL)

		err = svc.TriggerTestPing(ctx, creatorID, webhookID)
		require.NoError(t, err)

		deliveries, err := svc.ListDeliveries(ctx, creatorID, webhookID, 10, 0)
		require.NoError(t, err)
		assert.Len(t, deliveries, 1)

		err = svc.DeleteWebhook(ctx, creatorID, webhookID)
		require.NoError(t, err)
	})

	// 3. Peer Editor: DENY (404) for all 5 operations
	t.Run("PeerEditor_DeniedAllOperations_404", func(t *testing.T) {
		_, err := svc.GetWebhook(ctx, peerEditorID, webhookID)
		assert.ErrorIs(t, err, domain.ErrWebhookNotFound)

		_, err = svc.UpdateWebhook(ctx, peerEditorID, webhookID, updateReq)
		assert.ErrorIs(t, err, domain.ErrWebhookNotFound)

		err = svc.TriggerTestPing(ctx, peerEditorID, webhookID)
		assert.ErrorIs(t, err, domain.ErrWebhookNotFound)

		_, err = svc.ListDeliveries(ctx, peerEditorID, webhookID, 10, 0)
		assert.ErrorIs(t, err, domain.ErrWebhookNotFound)

		err = svc.DeleteWebhook(ctx, peerEditorID, webhookID)
		assert.ErrorIs(t, err, domain.ErrWebhookNotFound)
	})

	// 4. Project Viewer: DENY (404) for all 5 operations
	t.Run("ProjectViewer_DeniedAllOperations_404", func(t *testing.T) {
		_, err := svc.GetWebhook(ctx, viewerID, webhookID)
		assert.ErrorIs(t, err, domain.ErrWebhookNotFound)

		_, err = svc.UpdateWebhook(ctx, viewerID, webhookID, updateReq)
		assert.ErrorIs(t, err, domain.ErrWebhookNotFound)

		err = svc.TriggerTestPing(ctx, viewerID, webhookID)
		assert.ErrorIs(t, err, domain.ErrWebhookNotFound)

		_, err = svc.ListDeliveries(ctx, viewerID, webhookID, 10, 0)
		assert.ErrorIs(t, err, domain.ErrWebhookNotFound)

		err = svc.DeleteWebhook(ctx, viewerID, webhookID)
		assert.ErrorIs(t, err, domain.ErrWebhookNotFound)
	})

	// 5. Former Member (Removed from project): DENY (404) for all 5 operations
	t.Run("FormerMember_DeniedAllOperations_404", func(t *testing.T) {
		_, err := svc.GetWebhook(ctx, formerMemberID, webhookID)
		assert.ErrorIs(t, err, domain.ErrWebhookNotFound)

		_, err = svc.UpdateWebhook(ctx, formerMemberID, webhookID, updateReq)
		assert.ErrorIs(t, err, domain.ErrWebhookNotFound)

		err = svc.TriggerTestPing(ctx, formerMemberID, webhookID)
		assert.ErrorIs(t, err, domain.ErrWebhookNotFound)

		_, err = svc.ListDeliveries(ctx, formerMemberID, webhookID, 10, 0)
		assert.ErrorIs(t, err, domain.ErrWebhookNotFound)

		err = svc.DeleteWebhook(ctx, formerMemberID, webhookID)
		assert.ErrorIs(t, err, domain.ErrWebhookNotFound)
	})

	// 6. Creator Demoted to Viewer: DENY (404) even for their own webhook
	t.Run("DemotedCreator_DeniedAllOperations_404", func(t *testing.T) {
		_, err := svc.GetWebhook(ctx, demotedCreatorID, demotedWebhookID)
		assert.ErrorIs(t, err, domain.ErrWebhookNotFound)

		_, err = svc.UpdateWebhook(ctx, demotedCreatorID, demotedWebhookID, updateReq)
		assert.ErrorIs(t, err, domain.ErrWebhookNotFound)

		err = svc.TriggerTestPing(ctx, demotedCreatorID, demotedWebhookID)
		assert.ErrorIs(t, err, domain.ErrWebhookNotFound)

		_, err = svc.ListDeliveries(ctx, demotedCreatorID, demotedWebhookID, 10, 0)
		assert.ErrorIs(t, err, domain.ErrWebhookNotFound)

		err = svc.DeleteWebhook(ctx, demotedCreatorID, demotedWebhookID)
		assert.ErrorIs(t, err, domain.ErrWebhookNotFound)
	})

	// 7. Project Owner can manage demoted member's inactive webhook
	t.Run("ProjectOwner_CanManageDemotedMembersWebhook", func(t *testing.T) {
		res, err := svc.GetWebhook(ctx, ownerID, demotedWebhookID)
		require.NoError(t, err)
		assert.Equal(t, demotedWebhookID, res.ID)
		assert.False(t, res.IsActive, "webhook remains deactivated")

		// Owner can re-activate or update
		active := true
		upRes, err := svc.UpdateWebhook(ctx, ownerID, demotedWebhookID, UpdateWebhookRequest{IsActive: &active})
		require.NoError(t, err)
		assert.NotNil(t, upRes)
	})

	// 8. ListWebhooks Filtering
	t.Run("ListWebhooks_RoleFiltering", func(t *testing.T) {
		// Owner sees all project webhooks (2)
		ownerList, err := svc.ListWebhooks(ctx, ownerID, &projectID)
		require.NoError(t, err)
		assert.Len(t, ownerList, 2)

		// Creator editor sees only their own webhook (1)
		creatorList, err := svc.ListWebhooks(ctx, creatorID, &projectID)
		require.NoError(t, err)
		assert.Len(t, creatorList, 1)
		assert.Equal(t, webhookID, creatorList[0].ID)

		// Peer editor has 0 webhooks created by them
		peerList, err := svc.ListWebhooks(ctx, peerEditorID, &projectID)
		require.NoError(t, err)
		assert.Empty(t, peerList)

		// Viewer is rejected with 403
		_, err = svc.ListWebhooks(ctx, viewerID, &projectID)
		assert.ErrorIs(t, err, domain.ErrInsufficientProjectRole)
	})
}
