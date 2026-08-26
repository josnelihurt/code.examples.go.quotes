package infrastructure_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/josnelihurt/code.examples.go.quotes/internal/auth/domain"
	"github.com/josnelihurt/code.examples.go.quotes/internal/auth/infrastructure"
	"github.com/josnelihurt/code.examples.go.quotes/internal/platform/config"
)

func TestTheMaintainerHoldsBothScopes(t *testing.T) {
	store, err := infrastructure.NewHardcodedCredentialStore(config.EnvironmentDevelopment)
	require.NoError(t, err)

	decision, err := store.Validate(context.Background(), "jrb", "supersecret")
	require.NoError(t, err)

	assert.True(t, decision.Valid)
	assert.ElementsMatch(t, []string{domain.ScopeQuotesRead, domain.ScopeQuotesWrite}, decision.Scopes)
}

func TestTheReaderHoldsOnlyTheReadScope(t *testing.T) {
	store, err := infrastructure.NewHardcodedCredentialStore(config.EnvironmentDevelopment)
	require.NoError(t, err)

	decision, err := store.Validate(context.Background(), "reader", "readsecret")
	require.NoError(t, err)

	assert.True(t, decision.Valid)
	assert.ElementsMatch(t, []string{domain.ScopeQuotesRead}, decision.Scopes)
}

func TestAWrongPasswordIsInvalid(t *testing.T) {
	store, err := infrastructure.NewHardcodedCredentialStore(config.EnvironmentDevelopment)
	require.NoError(t, err)

	decision, err := store.Validate(context.Background(), "jrb", "wrong")
	require.NoError(t, err)

	assert.False(t, decision.Valid)
	assert.Empty(t, decision.Scopes)
}

func TestAnUnknownUserIsIndistinguishableFromAWrongPassword(t *testing.T) {
	store, err := infrastructure.NewHardcodedCredentialStore(config.EnvironmentDevelopment)
	require.NoError(t, err)

	unknown, err := store.Validate(context.Background(), "nobody", "supersecret")
	require.NoError(t, err)

	wrongPassword, err := store.Validate(context.Background(), "jrb", "readsecret")
	require.NoError(t, err)

	assert.Equal(t, unknown, wrongPassword, "the two failures cannot be told apart")
	assert.False(t, unknown.Valid)
}

func TestTheStoreRefusesToBootInProduction(t *testing.T) {
	_, err := infrastructure.NewHardcodedCredentialStore(config.EnvironmentProduction)

	require.ErrorIs(t, err, infrastructure.ErrProductionCredentialStore)
}
