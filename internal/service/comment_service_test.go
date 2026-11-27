package service

import (
	"context"
	"task-manager-api/internal/repository"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockCommentQuerier struct {
	mockQuerier
	getTaskByIDFn            func(ctx context.Context, id uuid.UUID) (repository.Task, error)
	getProjectByIDFn         func(ctx context.Context, id uuid.UUID) (repository.Project, error)
	getProjectMemberFn       func(ctx context.Context, arg repository.GetProjectMemberParams) (repository.ProjectMember, error)
	getUserByIDFn            func(ctx context.Context, id uuid.UUID) (repository.User, error)
	createTaskCommentFn      func(ctx context.Context, arg repository.CreateTaskCommentParams) (repository.TaskComment, error)
	getTaskCommentByIDFn     func(ctx context.Context, id uuid.UUID) (repository.GetTaskCommentByIDRow, error)
	listTaskCommentsFn       func(ctx context.Context, arg repository.ListTaskCommentsParams) ([]repository.ListTaskCommentsRow, error)
	listTaskCommentsKeysetFn func(ctx context.Context, arg repository.ListTaskCommentsKeysetParams) ([]repository.ListTaskCommentsKeysetRow, error)
	countTaskCommentsFn      func(ctx context.Context, taskID uuid.UUID) (int64, error)
	updateTaskCommentFn      func(ctx context.Context, arg repository.UpdateTaskCommentParams) (repository.TaskComment, error)
	deleteTaskCommentFn      func(ctx context.Context, arg repository.DeleteTaskCommentParams) error
}

func (m *mockCommentQuerier) GetTaskByID(ctx context.Context, id uuid.UUID) (repository.Task, error) {
	if m.getTaskByIDFn != nil {
		return m.getTaskByIDFn(ctx, id)
	}
	return repository.Task{}, pgx.ErrNoRows
}

func (m *mockCommentQuerier) GetProjectByID(ctx context.Context, id uuid.UUID) (repository.Project, error) {
	if m.getProjectByIDFn != nil {
		return m.getProjectByIDFn(ctx, id)
	}
	return repository.Project{}, pgx.ErrNoRows
}

func (m *mockCommentQuerier) GetProjectMember(ctx context.Context, arg repository.GetProjectMemberParams) (repository.ProjectMember, error) {
	if m.getProjectMemberFn != nil {
		return m.getProjectMemberFn(ctx, arg)
	}
	return repository.ProjectMember{}, pgx.ErrNoRows
}

func (m *mockCommentQuerier) GetUserByID(ctx context.Context, id uuid.UUID) (repository.User, error) {
	if m.getUserByIDFn != nil {
		return m.getUserByIDFn(ctx, id)
	}
	return repository.User{}, pgx.ErrNoRows
}

func (m *mockCommentQuerier) CreateTaskComment(ctx context.Context, arg repository.CreateTaskCommentParams) (repository.TaskComment, error) {
	if m.createTaskCommentFn != nil {
		return m.createTaskCommentFn(ctx, arg)
	}
	return repository.TaskComment{}, nil
}

func (m *mockCommentQuerier) GetTaskCommentByID(ctx context.Context, id uuid.UUID) (repository.GetTaskCommentByIDRow, error) {
	if m.getTaskCommentByIDFn != nil {
		return m.getTaskCommentByIDFn(ctx, id)
	}
	return repository.GetTaskCommentByIDRow{}, pgx.ErrNoRows
}

func (m *mockCommentQuerier) ListTaskComments(ctx context.Context, arg repository.ListTaskCommentsParams) ([]repository.ListTaskCommentsRow, error) {
	if m.listTaskCommentsFn != nil {
		return m.listTaskCommentsFn(ctx, arg)
	}
	return nil, nil
}

func (m *mockCommentQuerier) ListTaskCommentsKeyset(ctx context.Context, arg repository.ListTaskCommentsKeysetParams) ([]repository.ListTaskCommentsKeysetRow, error) {
	if m.listTaskCommentsKeysetFn != nil {
		return m.listTaskCommentsKeysetFn(ctx, arg)
	}
	return nil, nil
}

func (m *mockCommentQuerier) CountTaskComments(ctx context.Context, taskID uuid.UUID) (int64, error) {
	if m.countTaskCommentsFn != nil {
		return m.countTaskCommentsFn(ctx, taskID)
	}
	return 0, nil
}

func (m *mockCommentQuerier) UpdateTaskComment(ctx context.Context, arg repository.UpdateTaskCommentParams) (repository.TaskComment, error) {
	if m.updateTaskCommentFn != nil {
		return m.updateTaskCommentFn(ctx, arg)
	}
	return repository.TaskComment{}, pgx.ErrNoRows
}

func (m *mockCommentQuerier) DeleteTaskComment(ctx context.Context, arg repository.DeleteTaskCommentParams) error {
	if m.deleteTaskCommentFn != nil {
		return m.deleteTaskCommentFn(ctx, arg)
	}
	return nil
}

func TestCommentService_CreateComment(t *testing.T) {
	userID := uuid.New()
	taskID := uuid.New()
	commentID := uuid.New()

	task := repository.Task{
		ID:        taskID,
		CreatorID: userID,
		Title:     "Test Task",
	}

	user := repository.User{
		ID:       userID,
		Email:    "author@example.com",
		FullName: "Author User",
	}

	t.Run("Success", func(t *testing.T) {
		mock := &mockCommentQuerier{
			getTaskByIDFn: func(_ context.Context, _ uuid.UUID) (repository.Task, error) {
				return task, nil
			},
			createTaskCommentFn: func(_ context.Context, arg repository.CreateTaskCommentParams) (repository.TaskComment, error) {
				require.Equal(t, taskID, arg.TaskID)
				require.Equal(t, userID, arg.UserID)
				require.Equal(t, "This is a comment", arg.Content)
				return repository.TaskComment{
					ID:        commentID,
					TaskID:    arg.TaskID,
					UserID:    arg.UserID,
					Content:   arg.Content,
					CreatedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
					UpdatedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
				}, nil
			},
			getUserByIDFn: func(_ context.Context, _ uuid.UUID) (repository.User, error) {
				return user, nil
			},
		}

		svc := NewCommentService(mock)
		resp, err := svc.CreateComment(context.Background(), userID, taskID, CreateCommentInput{
			Content: "This is a comment",
		})

		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, commentID, resp.ID)
		require.Equal(t, "This is a comment", resp.Content)
		require.Equal(t, "Author User", resp.Author)
	})

	t.Run("EmptyContent", func(t *testing.T) {
		svc := NewCommentService(&mockCommentQuerier{})
		_, err := svc.CreateComment(context.Background(), userID, taskID, CreateCommentInput{
			Content: "   ",
		})
		require.ErrorIs(t, err, ErrEmptyCommentContent)
	})

	t.Run("TaskNotFound", func(t *testing.T) {
		mock := &mockCommentQuerier{
			getTaskByIDFn: func(_ context.Context, _ uuid.UUID) (repository.Task, error) {
				return repository.Task{}, pgx.ErrNoRows
			},
		}

		svc := NewCommentService(mock)
		_, err := svc.CreateComment(context.Background(), userID, taskID, CreateCommentInput{
			Content: "Valid comment",
		})
		require.ErrorIs(t, err, ErrTaskNotFound)
	})

	t.Run("AntiIDOR_UnauthorizedUser", func(t *testing.T) {
		otherUser := uuid.New()
		mock := &mockCommentQuerier{
			getTaskByIDFn: func(_ context.Context, _ uuid.UUID) (repository.Task, error) {
				return task, nil // owned by userID, but caller is otherUser
			},
		}

		svc := NewCommentService(mock)
		_, err := svc.CreateComment(context.Background(), otherUser, taskID, CreateCommentInput{
			Content: "Valid comment",
		})
		require.ErrorIs(t, err, ErrTaskNotFound)
	})
}

func TestCommentService_ListComments(t *testing.T) {
	userID := uuid.New()
	taskID := uuid.New()

	task := repository.Task{
		ID:        taskID,
		CreatorID: userID,
	}

	t.Run("Offset_Success", func(t *testing.T) {
		mock := &mockCommentQuerier{
			getTaskByIDFn: func(_ context.Context, _ uuid.UUID) (repository.Task, error) {
				return task, nil
			},
			countTaskCommentsFn: func(_ context.Context, _ uuid.UUID) (int64, error) {
				return 2, nil
			},
			listTaskCommentsFn: func(_ context.Context, arg repository.ListTaskCommentsParams) ([]repository.ListTaskCommentsRow, error) {
				require.Equal(t, int32(10), arg.Limit)
				require.Equal(t, int32(0), arg.Offset)
				return []repository.ListTaskCommentsRow{
					{ID: uuid.New(), TaskID: taskID, UserID: userID, Content: "Comment 1", FullName: "User 1", Email: "u1@test.com", CreatedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true}},
					{ID: uuid.New(), TaskID: taskID, UserID: userID, Content: "Comment 2", FullName: "User 2", Email: "u2@test.com", CreatedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true}},
				}, nil
			},
		}

		svc := NewCommentService(mock)
		resp, err := svc.ListComments(context.Background(), userID, taskID, PaginationParams{
			Page:  1,
			Limit: 10,
		})

		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, int64(2), resp.TotalItems)
		require.Len(t, resp.Items, 2)
		require.Equal(t, "Comment 1", resp.Items[0].Content)
	})

	t.Run("Keyset_Success", func(t *testing.T) {
		mock := &mockCommentQuerier{
			getTaskByIDFn: func(_ context.Context, _ uuid.UUID) (repository.Task, error) {
				return task, nil
			},
			listTaskCommentsKeysetFn: func(_ context.Context, _ repository.ListTaskCommentsKeysetParams) ([]repository.ListTaskCommentsKeysetRow, error) {
				return []repository.ListTaskCommentsKeysetRow{
					{ID: uuid.New(), TaskID: taskID, UserID: userID, Content: "Keyset comment", FullName: "User 1", Email: "u1@test.com", CreatedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true}},
				}, nil
			},
		}

		svc := NewCommentService(mock)
		resp, err := svc.ListComments(context.Background(), userID, taskID, PaginationParams{
			Cursor: "MjAyNi0xMS0wNlQxNjo1Mzo1My40NTAwMjVafDAxYTFiMWM3LWYyYWEtNzk5Zi1hY2FmLWMwNDM5OTZlOGE4OQ",
			Limit:  10,
		})

		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Len(t, resp.Items, 1)
	})
}

func TestCommentService_UpdateComment(t *testing.T) {
	authorID := uuid.New()
	otherUserID := uuid.New()
	taskID := uuid.New()
	commentID := uuid.New()

	task := repository.Task{
		ID:         taskID,
		CreatorID:  authorID,
		AssigneeID: pgtype.UUID{Bytes: otherUserID, Valid: true},
	}

	commentRow := repository.GetTaskCommentByIDRow{
		ID:        commentID,
		TaskID:    taskID,
		UserID:    authorID,
		Content:   "Original comment",
		Email:     "author@test.com",
		FullName:  "Author",
		CreatedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
		UpdatedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
	}

	t.Run("Success_AuthorUpdates", func(t *testing.T) {
		mock := &mockCommentQuerier{
			getTaskByIDFn: func(_ context.Context, _ uuid.UUID) (repository.Task, error) {
				return task, nil
			},
			getTaskCommentByIDFn: func(_ context.Context, _ uuid.UUID) (repository.GetTaskCommentByIDRow, error) {
				return commentRow, nil
			},
			updateTaskCommentFn: func(_ context.Context, arg repository.UpdateTaskCommentParams) (repository.TaskComment, error) {
				require.Equal(t, commentID, arg.ID)
				require.Equal(t, "Edited content", arg.Content)
				return repository.TaskComment{
					ID:        arg.ID,
					TaskID:    taskID,
					UserID:    authorID,
					Content:   arg.Content,
					CreatedAt: commentRow.CreatedAt,
					UpdatedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
				}, nil
			},
		}

		svc := NewCommentService(mock)
		resp, err := svc.UpdateComment(context.Background(), authorID, taskID, commentID, UpdateCommentInput{
			Content: "Edited content",
		})

		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, "Edited content", resp.Content)
	})

	t.Run("Forbidden_NotAuthor", func(t *testing.T) {
		mock := &mockCommentQuerier{
			getTaskByIDFn: func(_ context.Context, _ uuid.UUID) (repository.Task, error) {
				return task, nil
			},
			getTaskCommentByIDFn: func(_ context.Context, _ uuid.UUID) (repository.GetTaskCommentByIDRow, error) {
				return commentRow, nil // owned by authorID
			},
		}

		svc := NewCommentService(mock)
		_, err := svc.UpdateComment(context.Background(), otherUserID, taskID, commentID, UpdateCommentInput{
			Content: "Hacked content",
		})

		require.ErrorIs(t, err, ErrInsufficientProjectRole)
	})

	t.Run("EmptyContent", func(t *testing.T) {
		svc := NewCommentService(&mockCommentQuerier{})
		_, err := svc.UpdateComment(context.Background(), authorID, taskID, commentID, UpdateCommentInput{
			Content: "",
		})
		require.ErrorIs(t, err, ErrEmptyCommentContent)
	})
}

func TestCommentService_DeleteComment(t *testing.T) {
	authorID := uuid.New()
	projectOwnerID := uuid.New()
	randomUserID := uuid.New()
	taskID := uuid.New()
	commentID := uuid.New()

	projectID := uuid.New()
	task := repository.Task{
		ID:         taskID,
		CreatorID:  projectOwnerID,
		AssigneeID: pgtype.UUID{Bytes: authorID, Valid: true},
		ProjectID:  pgtype.UUID{Bytes: projectID, Valid: true},
	}

	commentRow := repository.GetTaskCommentByIDRow{
		ID:       commentID,
		TaskID:   taskID,
		UserID:   authorID,
		Content:  "Comment to delete",
		FullName: "Author",
		Email:    "author@test.com",
	}

	t.Run("AuthorDeletes_Success", func(t *testing.T) {
		deleted := false
		mock := &mockCommentQuerier{
			getTaskByIDFn: func(_ context.Context, _ uuid.UUID) (repository.Task, error) {
				return task, nil
			},
			getProjectByIDFn: func(_ context.Context, _ uuid.UUID) (repository.Project, error) {
				return repository.Project{ID: projectID, OwnerID: projectOwnerID}, nil
			},
			getProjectMemberFn: func(_ context.Context, arg repository.GetProjectMemberParams) (repository.ProjectMember, error) {
				return repository.ProjectMember{ProjectID: arg.ProjectID, UserID: arg.UserID, Role: "editor"}, nil
			},
			getTaskCommentByIDFn: func(_ context.Context, _ uuid.UUID) (repository.GetTaskCommentByIDRow, error) {
				return commentRow, nil
			},
			deleteTaskCommentFn: func(_ context.Context, arg repository.DeleteTaskCommentParams) error {
				require.Equal(t, commentID, arg.ID)
				require.Equal(t, taskID, arg.TaskID)
				deleted = true
				return nil
			},
		}

		svc := NewCommentService(mock)
		err := svc.DeleteComment(context.Background(), authorID, taskID, commentID)
		require.NoError(t, err)
		require.True(t, deleted)
	})

	t.Run("ProjectOwnerModerates_Success", func(t *testing.T) {
		deleted := false
		mock := &mockCommentQuerier{
			getTaskByIDFn: func(_ context.Context, _ uuid.UUID) (repository.Task, error) {
				return task, nil
			},
			getProjectByIDFn: func(_ context.Context, _ uuid.UUID) (repository.Project, error) {
				return repository.Project{ID: projectID, OwnerID: projectOwnerID}, nil
			},
			getTaskCommentByIDFn: func(_ context.Context, _ uuid.UUID) (repository.GetTaskCommentByIDRow, error) {
				return commentRow, nil
			},
			deleteTaskCommentFn: func(_ context.Context, arg repository.DeleteTaskCommentParams) error {
				require.Equal(t, commentID, arg.ID)
				require.Equal(t, taskID, arg.TaskID)
				deleted = true
				return nil
			},
		}

		svc := NewCommentService(mock)
		err := svc.DeleteComment(context.Background(), projectOwnerID, taskID, commentID)
		require.NoError(t, err)
		require.True(t, deleted)
	})

	t.Run("PersonalTaskCreatorModerates_Success", func(t *testing.T) {
		personalTaskID := uuid.New()
		personalCreatorID := uuid.New()
		personalTask := repository.Task{
			ID:        personalTaskID,
			CreatorID: personalCreatorID,
		}
		personalCommentRow := commentRow
		personalCommentRow.TaskID = personalTaskID
		deleted := false
		mock := &mockCommentQuerier{
			getTaskByIDFn: func(_ context.Context, _ uuid.UUID) (repository.Task, error) {
				return personalTask, nil
			},
			getTaskCommentByIDFn: func(_ context.Context, _ uuid.UUID) (repository.GetTaskCommentByIDRow, error) {
				return personalCommentRow, nil
			},
			deleteTaskCommentFn: func(_ context.Context, arg repository.DeleteTaskCommentParams) error {
				require.Equal(t, commentID, arg.ID)
				require.Equal(t, personalTaskID, arg.TaskID)
				deleted = true
				return nil
			},
		}

		svc := NewCommentService(mock)
		err := svc.DeleteComment(context.Background(), personalCreatorID, personalTaskID, commentID)
		require.NoError(t, err)
		require.True(t, deleted)
	})

	t.Run("Forbidden_RandomUser", func(t *testing.T) {
		mock := &mockCommentQuerier{
			getTaskByIDFn: func(_ context.Context, _ uuid.UUID) (repository.Task, error) {
				return task, nil
			},
			getProjectMemberFn: func(_ context.Context, arg repository.GetProjectMemberParams) (repository.ProjectMember, error) {
				return repository.ProjectMember{ProjectID: arg.ProjectID, UserID: arg.UserID, Role: "viewer"}, nil
			},
			getProjectByIDFn: func(_ context.Context, _ uuid.UUID) (repository.Project, error) {
				return repository.Project{ID: projectID, OwnerID: projectOwnerID}, nil
			},
			getTaskCommentByIDFn: func(_ context.Context, _ uuid.UUID) (repository.GetTaskCommentByIDRow, error) {
				return commentRow, nil
			},
		}

		svc := NewCommentService(mock)
		err := svc.DeleteComment(context.Background(), randomUserID, taskID, commentID)
		require.ErrorIs(t, err, ErrInsufficientProjectRole)
	})
}

func TestCommentService_ADR018_ViewerPermissions(t *testing.T) {
	viewerID := uuid.New()
	ownerID := uuid.New()
	otherMemberID := uuid.New()
	taskID := uuid.New()
	projectID := uuid.New()
	commentID := uuid.New()

	task := repository.Task{
		ID:        taskID,
		CreatorID: ownerID,
		ProjectID: pgtype.UUID{Bytes: projectID, Valid: true},
	}

	project := repository.Project{
		ID:      projectID,
		OwnerID: ownerID,
	}

	mockBase := func() *mockCommentQuerier {
		return &mockCommentQuerier{
			getTaskByIDFn: func(_ context.Context, _ uuid.UUID) (repository.Task, error) {
				return task, nil
			},
			getProjectByIDFn: func(_ context.Context, _ uuid.UUID) (repository.Project, error) {
				return project, nil
			},
			getProjectMemberFn: func(_ context.Context, arg repository.GetProjectMemberParams) (repository.ProjectMember, error) {
				if arg.UserID == viewerID {
					return repository.ProjectMember{ProjectID: projectID, UserID: viewerID, Role: "viewer"}, nil
				}
				return repository.ProjectMember{}, pgx.ErrNoRows
			},
			getUserByIDFn: func(_ context.Context, id uuid.UUID) (repository.User, error) {
				return repository.User{ID: id, FullName: "Viewer", Email: "viewer@example.com"}, nil
			},
		}
	}

	t.Run("Viewer_CanCreateComment", func(t *testing.T) {
		mock := mockBase()
		mock.createTaskCommentFn = func(_ context.Context, arg repository.CreateTaskCommentParams) (repository.TaskComment, error) {
			return repository.TaskComment{
				ID:        commentID,
				TaskID:    taskID,
				UserID:    viewerID,
				Content:   arg.Content,
				CreatedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
				UpdatedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
			}, nil
		}

		svc := NewCommentService(mock)
		resp, err := svc.CreateComment(context.Background(), viewerID, taskID, CreateCommentInput{
			Content: "Viewer comment",
		})
		require.NoError(t, err)
		require.Equal(t, "Viewer comment", resp.Content)
	})

	t.Run("Viewer_CanListComments", func(t *testing.T) {
		mock := mockBase()
		mock.countTaskCommentsFn = func(_ context.Context, _ uuid.UUID) (int64, error) {
			return 1, nil
		}
		mock.listTaskCommentsFn = func(_ context.Context, _ repository.ListTaskCommentsParams) ([]repository.ListTaskCommentsRow, error) {
			return []repository.ListTaskCommentsRow{
				{ID: commentID, TaskID: taskID, UserID: viewerID, Content: "Hello", FullName: "Viewer", Email: "v@v.com"},
			}, nil
		}

		svc := NewCommentService(mock)
		resp, err := svc.ListComments(context.Background(), viewerID, taskID, PaginationParams{Page: 1, Limit: 10})
		require.NoError(t, err)
		require.Len(t, resp.Items, 1)
	})

	t.Run("Viewer_CanDeleteOwnComment", func(t *testing.T) {
		deleted := false
		mock := mockBase()
		mock.getTaskCommentByIDFn = func(_ context.Context, _ uuid.UUID) (repository.GetTaskCommentByIDRow, error) {
			return repository.GetTaskCommentByIDRow{
				ID:     commentID,
				TaskID: taskID,
				UserID: viewerID, // authored by viewer
			}, nil
		}
		mock.deleteTaskCommentFn = func(_ context.Context, _ repository.DeleteTaskCommentParams) error {
			deleted = true
			return nil
		}

		svc := NewCommentService(mock)
		err := svc.DeleteComment(context.Background(), viewerID, taskID, commentID)
		require.NoError(t, err)
		require.True(t, deleted)
	})

	t.Run("Viewer_CannotDeleteOtherMemberComment", func(t *testing.T) {
		mock := mockBase()
		mock.getTaskCommentByIDFn = func(_ context.Context, _ uuid.UUID) (repository.GetTaskCommentByIDRow, error) {
			return repository.GetTaskCommentByIDRow{
				ID:     commentID,
				TaskID: taskID,
				UserID: otherMemberID, // authored by someone else
			}, nil
		}

		svc := NewCommentService(mock)
		err := svc.DeleteComment(context.Background(), viewerID, taskID, commentID)
		require.ErrorIs(t, err, ErrInsufficientProjectRole)
	})
}

func TestCommentService_ADR018_EditorCannotModerate(t *testing.T) {
	editorID := uuid.New()
	ownerID := uuid.New()
	authorID := uuid.New()
	taskID := uuid.New()
	projectID := uuid.New()
	commentID := uuid.New()

	task := repository.Task{
		ID:        taskID,
		CreatorID: ownerID,
		ProjectID: pgtype.UUID{Bytes: projectID, Valid: true},
	}

	project := repository.Project{
		ID:      projectID,
		OwnerID: ownerID,
	}

	mock := &mockCommentQuerier{
		getTaskByIDFn: func(_ context.Context, _ uuid.UUID) (repository.Task, error) {
			return task, nil
		},
		getProjectByIDFn: func(_ context.Context, _ uuid.UUID) (repository.Project, error) {
			return project, nil
		},
		getProjectMemberFn: func(_ context.Context, arg repository.GetProjectMemberParams) (repository.ProjectMember, error) {
			if arg.UserID == editorID {
				return repository.ProjectMember{ProjectID: projectID, UserID: editorID, Role: "editor"}, nil
			}
			return repository.ProjectMember{}, pgx.ErrNoRows
		},
		getTaskCommentByIDFn: func(_ context.Context, _ uuid.UUID) (repository.GetTaskCommentByIDRow, error) {
			return repository.GetTaskCommentByIDRow{
				ID:     commentID,
				TaskID: taskID,
				UserID: authorID, // different member
			}, nil
		},
	}

	svc := NewCommentService(mock)
	err := svc.DeleteComment(context.Background(), editorID, taskID, commentID)
	require.ErrorIs(t, err, ErrInsufficientProjectRole)
}

func TestCommentService_ADR018_OwnerCanModerate(t *testing.T) {
	ownerID := uuid.New()
	authorID := uuid.New()
	taskID := uuid.New()
	projectID := uuid.New()
	commentID := uuid.New()

	task := repository.Task{
		ID:        taskID,
		CreatorID: authorID,
		ProjectID: pgtype.UUID{Bytes: projectID, Valid: true},
	}

	project := repository.Project{
		ID:      projectID,
		OwnerID: ownerID,
	}

	deleted := false
	mock := &mockCommentQuerier{
		getTaskByIDFn: func(_ context.Context, _ uuid.UUID) (repository.Task, error) {
			return task, nil
		},
		getProjectByIDFn: func(_ context.Context, _ uuid.UUID) (repository.Project, error) {
			return project, nil
		},
		getTaskCommentByIDFn: func(_ context.Context, _ uuid.UUID) (repository.GetTaskCommentByIDRow, error) {
			return repository.GetTaskCommentByIDRow{
				ID:     commentID,
				TaskID: taskID,
				UserID: authorID, // different member
			}, nil
		},
		deleteTaskCommentFn: func(_ context.Context, _ repository.DeleteTaskCommentParams) error {
			deleted = true
			return nil
		},
	}

	svc := NewCommentService(mock)
	err := svc.DeleteComment(context.Background(), ownerID, taskID, commentID)
	require.NoError(t, err)
	require.True(t, deleted)
}

func TestCommentService_ADR018_RemovedCreatorGets404(t *testing.T) {
	removedCreatorID := uuid.New()
	ownerID := uuid.New()
	taskID := uuid.New()
	projectID := uuid.New()
	commentID := uuid.New()

	task := repository.Task{
		ID:        taskID,
		CreatorID: removedCreatorID, // Original creator
		ProjectID: pgtype.UUID{Bytes: projectID, Valid: true},
	}

	project := repository.Project{
		ID:      projectID,
		OwnerID: ownerID, // User is not owner
	}

	// User is no longer in project_members
	mock := &mockCommentQuerier{
		getTaskByIDFn: func(_ context.Context, _ uuid.UUID) (repository.Task, error) {
			return task, nil
		},
		getProjectByIDFn: func(_ context.Context, _ uuid.UUID) (repository.Project, error) {
			return project, nil
		},
		getProjectMemberFn: func(_ context.Context, _ repository.GetProjectMemberParams) (repository.ProjectMember, error) {
			return repository.ProjectMember{}, pgx.ErrNoRows // Removed!
		},
	}

	svc := NewCommentService(mock)

	t.Run("CreateComment_Returns404", func(t *testing.T) {
		_, err := svc.CreateComment(context.Background(), removedCreatorID, taskID, CreateCommentInput{Content: "Hi"})
		require.ErrorIs(t, err, ErrTaskNotFound)
	})

	t.Run("ListComments_Returns404", func(t *testing.T) {
		_, err := svc.ListComments(context.Background(), removedCreatorID, taskID, PaginationParams{})
		require.ErrorIs(t, err, ErrTaskNotFound)
	})

	t.Run("UpdateComment_Returns404", func(t *testing.T) {
		_, err := svc.UpdateComment(context.Background(), removedCreatorID, taskID, commentID, UpdateCommentInput{Content: "Update"})
		require.ErrorIs(t, err, ErrTaskNotFound)
	})

	t.Run("DeleteComment_Returns404", func(t *testing.T) {
		err := svc.DeleteComment(context.Background(), removedCreatorID, taskID, commentID)
		require.ErrorIs(t, err, ErrTaskNotFound)
	})
}

func TestCommentService_ADR018_SoftDeletedProjectGets404(t *testing.T) {
	ownerID := uuid.New()
	taskID := uuid.New()
	projectID := uuid.New()
	commentID := uuid.New()

	task := repository.Task{
		ID:        taskID,
		CreatorID: ownerID,
		ProjectID: pgtype.UUID{Bytes: projectID, Valid: true},
	}

	project := repository.Project{
		ID:        projectID,
		OwnerID:   ownerID,
		DeletedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true}, // Soft-deleted!
	}

	mock := &mockCommentQuerier{
		getTaskByIDFn: func(_ context.Context, _ uuid.UUID) (repository.Task, error) {
			return task, nil
		},
		getProjectByIDFn: func(_ context.Context, _ uuid.UUID) (repository.Project, error) {
			return project, nil
		},
	}

	svc := NewCommentService(mock)

	t.Run("CreateComment_Returns404", func(t *testing.T) {
		_, err := svc.CreateComment(context.Background(), ownerID, taskID, CreateCommentInput{Content: "Hi"})
		require.ErrorIs(t, err, ErrTaskNotFound)
	})

	t.Run("ListComments_Returns404", func(t *testing.T) {
		_, err := svc.ListComments(context.Background(), ownerID, taskID, PaginationParams{})
		require.ErrorIs(t, err, ErrTaskNotFound)
	})

	t.Run("UpdateComment_Returns404", func(t *testing.T) {
		_, err := svc.UpdateComment(context.Background(), ownerID, taskID, commentID, UpdateCommentInput{Content: "Update"})
		require.ErrorIs(t, err, ErrTaskNotFound)
	})

	t.Run("DeleteComment_Returns404", func(t *testing.T) {
		err := svc.DeleteComment(context.Background(), ownerID, taskID, commentID)
		require.ErrorIs(t, err, ErrTaskNotFound)
	})
}

func TestCommentService_OutboxIntegration(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()
	taskID := uuid.New()
	commentID := uuid.New()

	mock := &mockCommentQuerier{
		getTaskByIDFn: func(_ context.Context, id uuid.UUID) (repository.Task, error) {
			return repository.Task{
				ID:        id,
				CreatorID: userID,
			}, nil
		},
		createTaskCommentFn: func(_ context.Context, arg repository.CreateTaskCommentParams) (repository.TaskComment, error) {
			return repository.TaskComment{
				ID:      commentID,
				TaskID:  arg.TaskID,
				UserID:  arg.UserID,
				Content: arg.Content,
			}, nil
		},
		getUserByIDFn: func(_ context.Context, id uuid.UUID) (repository.User, error) {
			return repository.User{
				ID:       id,
				FullName: "Commenter User",
			}, nil
		},
	}

	svc := NewCommentService(mock)
	res, err := svc.CreateComment(ctx, userID, taskID, CreateCommentInput{
		Content: "Outbox Comment Content",
	})
	require.NoError(t, err)
	require.NotNil(t, res)

	require.Len(t, mock.outboxEvents, 1)
	evt := mock.outboxEvents[0]
	assert.Equal(t, "comment.created", evt.EventType)
	assert.Equal(t, "comment", evt.AggregateType)
	assert.Equal(t, commentID, evt.AggregateID)
	assert.Equal(t, userID, evt.ActorID)
	assert.Contains(t, string(evt.Payload), "Outbox Comment Content")
}
