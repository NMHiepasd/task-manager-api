//go:build integration

package repository_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"task-manager-api/internal/repository"
)

func TestIntegration_AccountLockout_Queries(t *testing.T) {
	ctx := context.Background()

	// 1. Create a user with 'auditor' role to verify migration constraint
	auditorEmail := fmt.Sprintf("auditor_%d@example.com", time.Now().UnixNano())
	auditor, err := testStore.CreateUser(ctx, repository.CreateUserParams{
		Email:        auditorEmail,
		PasswordHash: "hashed_auditor_password",
		FullName:     "System Auditor",
		Role:         "auditor",
	})
	require.NoError(t, err, "Migration 000011 must allow 'auditor' role in users table")
	assert.Equal(t, "auditor", auditor.Role)

	// 2. Create a standard user for lockout testing
	testEmail := fmt.Sprintf("lockout_user_%d@example.com", time.Now().UnixNano())
	user, err := testStore.CreateUser(ctx, repository.CreateUserParams{
		Email:        testEmail,
		PasswordHash: "hashed_dummy_password",
		FullName:     "Lockout Test User",
		Role:         "user",
	})
	require.NoError(t, err)

	// Initial state: 0 failed attempts, locked_until is NULL
	freshUser, err := testStore.GetUserByID(ctx, user.ID)
	require.NoError(t, err)
	assert.Equal(t, int32(0), freshUser.FailedLoginAttempts)
	assert.False(t, freshUser.LockedUntil.Valid)

	// 3. Record failed login attempts 1 through 4
	for attempt := int32(1); attempt <= 4; attempt++ {
		row, err := testStore.RecordFailedLogin(ctx, user.ID)
		require.NoError(t, err)
		assert.Equal(t, attempt, row.FailedLoginAttempts)
		assert.False(t, row.LockedUntil.Valid, "Account must not be locked before 5 attempts")
	}

	// 4. 5th failed attempt -> account must be locked for 15 minutes
	row5, err := testStore.RecordFailedLogin(ctx, user.ID)
	require.NoError(t, err)
	assert.Equal(t, int32(5), row5.FailedLoginAttempts)
	require.True(t, row5.LockedUntil.Valid, "Account must be locked on 5th failed attempt")
	assert.WithinDuration(t, time.Now().Add(15*time.Minute), row5.LockedUntil.Time, 10*time.Second)

	// Verify persistence in DB
	lockedUser, err := testStore.GetUserByID(ctx, user.ID)
	require.NoError(t, err)
	assert.Equal(t, int32(5), lockedUser.FailedLoginAttempts)
	assert.True(t, lockedUser.LockedUntil.Valid)
	assert.True(t, lockedUser.LastFailedLoginAt.Valid)

	// 5. Reset failed login -> counter back to 0, locked_until cleared
	err = testStore.ResetFailedLogin(ctx, user.ID)
	require.NoError(t, err)

	unlockedUser, err := testStore.GetUserByID(ctx, user.ID)
	require.NoError(t, err)
	assert.Equal(t, int32(0), unlockedUser.FailedLoginAttempts)
	assert.False(t, unlockedUser.LockedUntil.Valid)
}
