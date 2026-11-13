package tracer

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInitTracer(t *testing.T) {
	ctx := context.Background()
	tp, cleanup, err := InitTracer(ctx, "test-service", "test")
	require.NoError(t, err)
	require.NotNil(t, tp)
	require.NotNil(t, cleanup)

	shutdownCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	err = cleanup(shutdownCtx)
	assert.NoError(t, err)
}
