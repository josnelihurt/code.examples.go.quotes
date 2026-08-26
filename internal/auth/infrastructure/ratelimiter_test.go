package infrastructure_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/josnelihurt/code.examples.go.quotes/internal/auth/infrastructure"
)

func TestRequestsBeyondTheWindowLimitAreRejected(t *testing.T) {
	sut := infrastructure.NewRateLimiter(2, time.Minute)

	assert.True(t, sut.Allow("192.0.2.1"), "the first request fits")
	assert.True(t, sut.Allow("192.0.2.1"), "the second request fits")
	assert.False(t, sut.Allow("192.0.2.1"), "the third is over the budget")
	assert.False(t, sut.Allow("192.0.2.1"), "and it stays rejected inside the window")
}

func TestTheWindowResetsOnceItElapses(t *testing.T) {
	sut := infrastructure.NewRateLimiter(1, 40*time.Millisecond)

	assert.True(t, sut.Allow("192.0.2.1"))
	assert.False(t, sut.Allow("192.0.2.1"))

	time.Sleep(60 * time.Millisecond)

	assert.True(t, sut.Allow("192.0.2.1"), "a new window opens after the old one expires")
}

func TestEachClientKeyGetsItsOwnWindow(t *testing.T) {
	sut := infrastructure.NewRateLimiter(1, time.Minute)

	assert.True(t, sut.Allow("192.0.2.1"))
	assert.False(t, sut.Allow("192.0.2.1"))
	assert.True(t, sut.Allow("192.0.2.2"), "another client is not penalized")
}

func TestTheLimiterIsSafeForConcurrentUse(t *testing.T) {
	const permits = 50
	sut := infrastructure.NewRateLimiter(permits, time.Minute)

	allowed := make(chan bool, permits*2)
	for range 2 * permits {
		go func() {
			allowed <- sut.Allow("192.0.2.1")
		}()
	}

	granted := 0
	for range 2 * permits {
		if <-allowed {
			granted++
		}
	}

	require.Equal(t, permits, granted, "exactly the permit budget is granted")
}
