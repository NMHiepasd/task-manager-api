package authz_test

import (
	"errors"
	"task-manager-api/internal/authz"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestADR018PermissionMatrix(t *testing.T) {
	// Setup UUIDs
	ownerID := uuid.New()
	editorID := uuid.New()
	viewerID := uuid.New()
	projectOutsiderID := uuid.New()

	personalCreatorID := uuid.New()
	personalAssigneeID := uuid.New()
	personalOutsiderID := uuid.New()

	projectID := uuid.New()
	taskID := uuid.New()

	// Define role test cases according to ADR-018
	type roleTestCase struct {
		name     string
		sub      authz.Subject
		res      authz.Resource
		expected map[authz.Action]bool
	}

	testCases := []roleTestCase{
		{
			name: "Project: owner",
			sub:  authz.Subject{UserID: ownerID},
			res: authz.Resource{
				TaskID:     taskID,
				ProjectID:  &projectID,
				CreatorID:  editorID,
				MemberRole: authz.RolePtr(authz.RoleOwner),
			},
			expected: map[authz.Action]bool{
				authz.ActionRead:                true,
				authz.ActionComment:             true,
				authz.ActionEdit:                true,
				authz.ActionDelete:              true,
				authz.ActionUpload:              true,
				authz.ActionModerateComments:    true,
				authz.ActionModerateAttachments: true,
			},
		},
		{
			name: "Project: editor",
			sub:  authz.Subject{UserID: editorID},
			res: authz.Resource{
				TaskID:     taskID,
				ProjectID:  &projectID,
				CreatorID:  ownerID,
				MemberRole: authz.RolePtr(authz.RoleEditor),
			},
			expected: map[authz.Action]bool{
				authz.ActionRead:                true,
				authz.ActionComment:             true,
				authz.ActionEdit:                true,
				authz.ActionDelete:              true,
				authz.ActionUpload:              true,
				authz.ActionModerateComments:    false,
				authz.ActionModerateAttachments: false,
			},
		},
		{
			name: "Project: viewer",
			sub:  authz.Subject{UserID: viewerID},
			res: authz.Resource{
				TaskID:     taskID,
				ProjectID:  &projectID,
				CreatorID:  ownerID,
				MemberRole: authz.RolePtr(authz.RoleViewer),
			},
			expected: map[authz.Action]bool{
				authz.ActionRead:                true,
				authz.ActionComment:             true,
				authz.ActionEdit:                false,
				authz.ActionDelete:              false,
				authz.ActionUpload:              false,
				authz.ActionModerateComments:    false,
				authz.ActionModerateAttachments: false,
			},
		},
		{
			name: "Project: outsider",
			sub:  authz.Subject{UserID: projectOutsiderID},
			res: authz.Resource{
				TaskID:     taskID,
				ProjectID:  &projectID,
				CreatorID:  ownerID,
				MemberRole: nil,
			},
			expected: map[authz.Action]bool{
				authz.ActionRead:                false,
				authz.ActionComment:             false,
				authz.ActionEdit:                false,
				authz.ActionDelete:              false,
				authz.ActionUpload:              false,
				authz.ActionModerateComments:    false,
				authz.ActionModerateAttachments: false,
			},
		},
		{
			name: "Personal: creator",
			sub:  authz.Subject{UserID: personalCreatorID},
			res: authz.Resource{
				TaskID:     taskID,
				ProjectID:  nil,
				CreatorID:  personalCreatorID,
				AssigneeID: &personalAssigneeID,
			},
			expected: map[authz.Action]bool{
				authz.ActionRead:                true,
				authz.ActionComment:             true,
				authz.ActionEdit:                true,
				authz.ActionDelete:              true,
				authz.ActionUpload:              true,
				authz.ActionModerateComments:    true,
				authz.ActionModerateAttachments: true,
			},
		},
		{
			name: "Personal: assignee",
			sub:  authz.Subject{UserID: personalAssigneeID},
			res: authz.Resource{
				TaskID:     taskID,
				ProjectID:  nil,
				CreatorID:  personalCreatorID,
				AssigneeID: &personalAssigneeID,
			},
			expected: map[authz.Action]bool{
				authz.ActionRead:                true,
				authz.ActionComment:             true,
				authz.ActionEdit:                false,
				authz.ActionDelete:              false,
				authz.ActionUpload:              true,
				authz.ActionModerateComments:    false,
				authz.ActionModerateAttachments: false,
			},
		},
		{
			name: "Personal: outsider",
			sub:  authz.Subject{UserID: personalOutsiderID},
			res: authz.Resource{
				TaskID:     taskID,
				ProjectID:  nil,
				CreatorID:  personalCreatorID,
				AssigneeID: &personalAssigneeID,
			},
			expected: map[authz.Action]bool{
				authz.ActionRead:                false,
				authz.ActionComment:             false,
				authz.ActionEdit:                false,
				authz.ActionDelete:              false,
				authz.ActionUpload:              false,
				authz.ActionModerateComments:    false,
				authz.ActionModerateAttachments: false,
			},
		},
	}

	allActions := []authz.Action{
		authz.ActionRead,
		authz.ActionComment,
		authz.ActionEdit,
		authz.ActionDelete,
		authz.ActionUpload,
		authz.ActionModerateComments,
		authz.ActionModerateAttachments,
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			for _, action := range allActions {
				expectedAllowed, ok := tc.expected[action]
				require.True(t, ok, "missing expected outcome for action %s", action)

				allowed := authz.Can(tc.sub, tc.res, action)
				assert.Equal(t, expectedAllowed, allowed, "action %s permission mismatch", action)

				err := authz.Authorize(tc.sub, tc.res, action)
				if expectedAllowed {
					assert.NoError(t, err, "expected Authorize to allow %s", action)
				} else {
					// Check Anti-IDOR: if caller lacks Read access, expect ErrNotFound.
					// Otherwise (has Read but lacks action), expect ErrForbidden.
					hasRead := tc.expected[authz.ActionRead]
					if !hasRead {
						assert.True(t, errors.Is(err, authz.ErrNotFound), "expected ErrNotFound for %s, got: %v", action, err)
					} else {
						assert.True(t, errors.Is(err, authz.ErrForbidden), "expected ErrForbidden for %s, got: %v", action, err)
					}
				}
			}
		})
	}
}

func TestProjectTasks_EdgeCases(t *testing.T) {
	projectID := uuid.New()
	taskID := uuid.New()
	creatorID := uuid.New()
	assigneeID := uuid.New()

	t.Run("soft-deleted project denies all actions with ErrNotFound", func(t *testing.T) {
		ownerID := uuid.New()
		res := authz.Resource{
			TaskID:         taskID,
			ProjectID:      &projectID,
			ProjectDeleted: true,
			CreatorID:      creatorID,
			MemberRole:     authz.RolePtr(authz.RoleOwner),
		}
		sub := authz.Subject{UserID: ownerID}

		for _, action := range []authz.Action{authz.ActionRead, authz.ActionEdit, authz.ActionDelete} {
			assert.False(t, authz.Can(sub, res, action))
			err := authz.Authorize(sub, res, action)
			assert.True(t, errors.Is(err, authz.ErrNotFound), "expected ErrNotFound, got %v", err)
		}
	})

	t.Run("creator removed from project loses all rights (Anti-IDOR)", func(t *testing.T) {
		// User is CreatorID on task, but removed from project (MemberRole is nil)
		res := authz.Resource{
			TaskID:     taskID,
			ProjectID:  &projectID,
			CreatorID:  creatorID,
			MemberRole: nil,
		}
		sub := authz.Subject{UserID: creatorID}

		assert.False(t, authz.Can(sub, res, authz.ActionRead))
		assert.False(t, authz.Can(sub, res, authz.ActionEdit))
		assert.False(t, authz.Can(sub, res, authz.ActionDelete))
		assert.False(t, authz.Can(sub, res, authz.ActionModerateComments))

		err := authz.Authorize(sub, res, authz.ActionRead)
		assert.True(t, errors.Is(err, authz.ErrNotFound))
		err = authz.Authorize(sub, res, authz.ActionEdit)
		assert.True(t, errors.Is(err, authz.ErrNotFound))
	})

	t.Run("assignee removed from project loses all rights (Anti-IDOR)", func(t *testing.T) {
		// User is AssigneeID on task, but removed from project (MemberRole is nil)
		res := authz.Resource{
			TaskID:     taskID,
			ProjectID:  &projectID,
			CreatorID:  creatorID,
			AssigneeID: &assigneeID,
			MemberRole: nil,
		}
		sub := authz.Subject{UserID: assigneeID}

		assert.False(t, authz.Can(sub, res, authz.ActionRead))
		assert.False(t, authz.Can(sub, res, authz.ActionEdit))
		err := authz.Authorize(sub, res, authz.ActionRead)
		assert.True(t, errors.Is(err, authz.ErrNotFound))
	})

	t.Run("assignee who is viewer in project task cannot edit", func(t *testing.T) {
		// Assignee is a viewer in project tasks; project role strictly applies
		res := authz.Resource{
			TaskID:     taskID,
			ProjectID:  &projectID,
			CreatorID:  creatorID,
			AssigneeID: &assigneeID,
			MemberRole: authz.RolePtr(authz.RoleViewer),
		}
		sub := authz.Subject{UserID: assigneeID}

		assert.True(t, authz.Can(sub, res, authz.ActionRead))
		assert.True(t, authz.Can(sub, res, authz.ActionComment))
		assert.False(t, authz.Can(sub, res, authz.ActionEdit))
		assert.False(t, authz.Can(sub, res, authz.ActionUpload))

		err := authz.Authorize(sub, res, authz.ActionEdit)
		assert.True(t, errors.Is(err, authz.ErrForbidden))
	})

	t.Run("unauthenticated nil user id is always denied", func(t *testing.T) {
		res := authz.Resource{
			TaskID:     taskID,
			ProjectID:  &projectID,
			CreatorID:  creatorID,
			MemberRole: authz.RolePtr(authz.RoleOwner),
		}
		sub := authz.Subject{UserID: uuid.Nil}

		assert.False(t, authz.Can(sub, res, authz.ActionRead))
		err := authz.Authorize(sub, res, authz.ActionRead)
		assert.True(t, errors.Is(err, authz.ErrNotFound))
	})

	t.Run("unrecognized role is denied", func(t *testing.T) {
		invalidRole := authz.ProjectRole("hacker")
		res := authz.Resource{
			TaskID:     taskID,
			ProjectID:  &projectID,
			CreatorID:  creatorID,
			MemberRole: &invalidRole,
		}
		sub := authz.Subject{UserID: uuid.New()}

		assert.False(t, authz.Can(sub, res, authz.ActionRead))
	})

	t.Run("unknown action is denied", func(t *testing.T) {
		res := authz.Resource{
			TaskID:     taskID,
			ProjectID:  &projectID,
			CreatorID:  creatorID,
			MemberRole: authz.RolePtr(authz.RoleOwner),
		}
		sub := authz.Subject{UserID: uuid.New()}

		assert.False(t, authz.Can(sub, res, authz.Action("Exploit")))
	})
}

func TestSubResource_SelfAuthoredAndModeration(t *testing.T) {
	projectID := uuid.New()
	taskID := uuid.New()
	ownerID := uuid.New()
	editorID := uuid.New()
	viewerID := uuid.New()
	authorID := uuid.New()
	outsiderID := uuid.New()

	baseResource := authz.Resource{
		TaskID:    taskID,
		ProjectID: &projectID,
		CreatorID: ownerID,
	}

	t.Run("comments moderation and self modification", func(t *testing.T) {
		// Case 1: Author has viewer access on task -> can edit/delete their own comment
		viewerRes := baseResource
		viewerRes.MemberRole = authz.RolePtr(authz.RoleViewer)
		subAuthorViewer := authz.Subject{UserID: authorID}
		assert.True(t, authz.CanModifyComment(subAuthorViewer, viewerRes, authorID))
		assert.NoError(t, authz.AuthorizeModifyComment(subAuthorViewer, viewerRes, authorID))

		// Case 2: Project owner can moderate other user's comment
		ownerRes := baseResource
		ownerRes.MemberRole = authz.RolePtr(authz.RoleOwner)
		subOwner := authz.Subject{UserID: ownerID}
		assert.True(t, authz.CanModifyComment(subOwner, ownerRes, authorID))
		assert.NoError(t, authz.AuthorizeModifyComment(subOwner, ownerRes, authorID))

		// Case 3: Project editor CANNOT moderate other user's comment
		editorRes := baseResource
		editorRes.MemberRole = authz.RolePtr(authz.RoleEditor)
		subEditor := authz.Subject{UserID: editorID}
		assert.False(t, authz.CanModifyComment(subEditor, editorRes, authorID))
		err := authz.AuthorizeModifyComment(subEditor, editorRes, authorID)
		assert.True(t, errors.Is(err, authz.ErrForbidden))

		// Case 4: Project viewer CANNOT moderate other user's comment
		subViewer := authz.Subject{UserID: viewerID}
		assert.False(t, authz.CanModifyComment(subViewer, viewerRes, authorID))
		err = authz.AuthorizeModifyComment(subViewer, viewerRes, authorID)
		assert.True(t, errors.Is(err, authz.ErrForbidden))

		// Case 5: Author removed from project -> cannot edit their comment (Anti-IDOR)
		outsiderRes := baseResource
		outsiderRes.MemberRole = nil
		assert.False(t, authz.CanModifyComment(subAuthorViewer, outsiderRes, authorID))
		err = authz.AuthorizeModifyComment(subAuthorViewer, outsiderRes, authorID)
		assert.True(t, errors.Is(err, authz.ErrNotFound))

		// Case 6: Complete outsider cannot moderate
		subOutsider := authz.Subject{UserID: outsiderID}
		assert.False(t, authz.CanModifyComment(subOutsider, outsiderRes, authorID))
		err = authz.AuthorizeModifyComment(subOutsider, outsiderRes, authorID)
		assert.True(t, errors.Is(err, authz.ErrNotFound))
	})

	t.Run("attachments moderation and self deletion", func(t *testing.T) {
		uploaderID := uuid.New()

		// Case 1: Uploader has editor access on task -> can delete their own attachment
		editorRes := baseResource
		editorRes.MemberRole = authz.RolePtr(authz.RoleEditor)
		subUploaderEditor := authz.Subject{UserID: uploaderID}
		assert.True(t, authz.CanDeleteAttachment(subUploaderEditor, editorRes, uploaderID))
		assert.NoError(t, authz.AuthorizeDeleteAttachment(subUploaderEditor, editorRes, uploaderID))

		// Case 2: Project owner can moderate other user's attachment
		ownerRes := baseResource
		ownerRes.MemberRole = authz.RolePtr(authz.RoleOwner)
		subOwner := authz.Subject{UserID: ownerID}
		assert.True(t, authz.CanDeleteAttachment(subOwner, ownerRes, uploaderID))
		assert.NoError(t, authz.AuthorizeDeleteAttachment(subOwner, ownerRes, uploaderID))

		// Case 3: Project editor CANNOT moderate other user's attachment
		subEditor := authz.Subject{UserID: editorID}
		assert.False(t, authz.CanDeleteAttachment(subEditor, editorRes, uploaderID))
		err := authz.AuthorizeDeleteAttachment(subEditor, editorRes, uploaderID)
		assert.True(t, errors.Is(err, authz.ErrForbidden))

		// Case 4: Uploader removed from project -> cannot delete their attachment (Anti-IDOR)
		outsiderRes := baseResource
		outsiderRes.MemberRole = nil
		assert.False(t, authz.CanDeleteAttachment(subUploaderEditor, outsiderRes, uploaderID))
		err = authz.AuthorizeDeleteAttachment(subUploaderEditor, outsiderRes, uploaderID)
		assert.True(t, errors.Is(err, authz.ErrNotFound))
	})
}

func TestParseRole(t *testing.T) {
	assert.Equal(t, authz.RolePtr(authz.RoleOwner), authz.ParseRole("owner"))
	assert.Equal(t, authz.RolePtr(authz.RoleEditor), authz.ParseRole("editor"))
	assert.Equal(t, authz.RolePtr(authz.RoleViewer), authz.ParseRole("viewer"))
	assert.Nil(t, authz.ParseRole(""))
	assert.Nil(t, authz.ParseRole("admin"))
	assert.Nil(t, authz.ParseRole("viewer "))
}
