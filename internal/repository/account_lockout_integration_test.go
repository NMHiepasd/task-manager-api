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
		row, err := testStore.RecordFailedLogin(ctx, repository.RecordFailedLoginParams{
			ID:              user.ID,
			Threshold:       5,
			DurationSeconds: 900,
			WindowSeconds:   900,
		})
		require.NoError(t, err)
		assert.Equal(t, attempt, row.FailedLoginAttempts)
		assert.False(t, row.LockedUntil.Valid, "Account must not be locked before 5 attempts")
	}

	// 4. 5th failed attempt -> account must be locked for 15 minutes
	row5, err := testStore.RecordFailedLogin(ctx, repository.RecordFailedLoginParams{
		ID:              user.ID,
		Threshold:       5,
		DurationSeconds: 900,
		WindowSeconds:   900,
	})
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

func TestIntegration_AccountLockout_SlidingWindowAndExpiration(t *testing.T) {
	ctx := context.Background()

	testEmail := fmt.Sprintf("lockout_window_%d@example.com", time.Now().UnixNano())
	user, err := testStore.CreateUser(ctx, repository.CreateUserParams{
		Email:        testEmail,
		PasswordHash: "hashed_dummy_password",
		FullName:     "Lockout Window User",
		Role:         "user",
	})
	require.NoError(t, err)

	// 1. Fail 4 times within window (1s)
	for attempt := int32(1); attempt <= 4; attempt++ {
		row, recErr := testStore.RecordFailedLogin(ctx, repository.RecordFailedLoginParams{
			ID:              user.ID,
			Threshold:       5,
			DurationSeconds: 1,
			WindowSeconds:   1,
		})
		require.NoError(t, recErr)
		assert.Equal(t, attempt, row.FailedLoginAttempts)
		assert.False(t, row.LockedUntil.Valid)
	}

	// Wait past the 1s window
	time.Sleep(1100 * time.Millisecond)

	// 5th attempt after window expired -> counter resets to 1, NOT locked
	rowAfterWindow, err := testStore.RecordFailedLogin(ctx, repository.RecordFailedLoginParams{
		ID:              user.ID,
		Threshold:       5,
		DurationSeconds: 1,
		WindowSeconds:   1,
	})
	require.NoError(t, err)
	assert.Equal(t, int32(1), rowAfterWindow.FailedLoginAttempts, "counter must reset to 1 when last_failed_login_at is older than window")
	assert.False(t, rowAfterWindow.LockedUntil.Valid, "account must not lock after window elapsed")

	// 2. Now fail 4 more times quickly (attempts 2..5) -> triggers 1s lock
	for attempt := int32(2); attempt <= 5; attempt++ {
		row, recErr := testStore.RecordFailedLogin(ctx, repository.RecordFailedLoginParams{
			ID:              user.ID,
			Threshold:       5,
			DurationSeconds: 1,
			WindowSeconds:   60,
		})
		require.NoError(t, recErr)
		assert.Equal(t, attempt, row.FailedLoginAttempts)
		if attempt == 5 {
			assert.True(t, row.LockedUntil.Valid, "must lock on 5th attempt")
		}
	}

	// Wait for the 1s lock to expire (still well within 60s window)
	time.Sleep(1100 * time.Millisecond)

	// Fail once after lock expired -> counter must reset to 1 and lock cleared
	rowAfterLockExpired, err := testStore.RecordFailedLogin(ctx, repository.RecordFailedLoginParams{
		ID:              user.ID,
		Threshold:       5,
		DurationSeconds: 1,
		WindowSeconds:   60,
	})
	require.NoError(t, err)
	assert.Equal(t, int32(1), rowAfterLockExpired.FailedLoginAttempts, "counter must reset to 1 after lock expires")
	assert.False(t, rowAfterLockExpired.LockedUntil.Valid)
}
