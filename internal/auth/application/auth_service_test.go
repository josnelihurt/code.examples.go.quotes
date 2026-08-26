package application_test

import (
	"context"
	"errors"
	"testing"

	"github.com/josnelihurt/code.examples.go.quotes/internal/auth/application"
	"github.com/josnelihurt/code.examples.go.quotes/internal/auth/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoginReturnsAResultWhenCredentialsAreAccepted(t *testing.T) {
	credentials := &stubCredentialStore{
		decision: domain.CredentialValidationResult{
			Valid:  true,
			Scopes: []string{domain.ScopeQuotesRead},
		},
	}
	tokens := &stubTokenIssuer{
		issued: domain.IssuedToken{AccessToken: "issued-token", ExpiresInSeconds: 900},
	}
	sut := application.NewAuthService(credentials, tokens)

	result, err := sut.Login(context.Background(), application.LoginRequest{
		Username: "jrb",
		Password: "secret",
	})

	require.NoError(t, err)
	assert.Equal(t, "issued-token", result.AccessToken)
	assert.Equal(t, "jrb", result.Username)
	assert.Equal(t, 900, result.ExpiresIn)
}

func TestLoginForwardsTheScopesTheStoreGrantedToTheTokenIssuer(t *testing.T) {
	credentials := &stubCredentialStore{
		decision: domain.CredentialValidationResult{
			Valid:  true,
			Scopes: []string{domain.ScopeQuotesRead, domain.ScopeQuotesWrite},
		},
	}
	tokens := &stubTokenIssuer{
		issued: domain.IssuedToken{AccessToken: "issued-token", ExpiresInSeconds: 900},
	}
	sut := application.NewAuthService(credentials, tokens)

	_, err := sut.Login(context.Background(), application.LoginRequest{
		Username: "reader",
		Password: "secret",
	})

	require.NoError(t, err)
	require.Len(t, tokens.createCalls, 1)
	assert.Equal(t, "reader", tokens.createCalls[0].username)
	assert.Contains(t, tokens.createCalls[0].scopes, domain.ScopeQuotesRead)
	assert.Contains(t, tokens.createCalls[0].scopes, domain.ScopeQuotesWrite)
}

func TestLoginReturnsInvalidCredentialsWhenTheStoreRejects(t *testing.T) {
	credentials := &stubCredentialStore{decision: domain.CredentialValidationResult{}}
	tokens := &stubTokenIssuer{}
	sut := application.NewAuthService(credentials, tokens)

	_, err := sut.Login(context.Background(), application.LoginRequest{
		Username: "jrb",
		Password: "wrong",
	})

	requireCode(t, err, "auth.invalid_credentials")
	assert.Empty(t, tokens.createCalls)
}

func TestLoginRejectsBlankInputWithoutTouchingTheCredentialStore(t *testing.T) {
	tests := []struct {
		name     string
		username string
		password string
	}{
		{name: "empty username", username: "", password: "secret"},
		{name: "empty password", username: "jrb", password: ""},
		{name: "blank username", username: "   ", password: "secret"},
		{name: "blank password", username: "jrb", password: "   "},
		{name: "both empty", username: "", password: ""},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			credentials := &stubCredentialStore{}
			tokens := &stubTokenIssuer{}
			sut := application.NewAuthService(credentials, tokens)

			_, err := sut.Login(context.Background(), application.LoginRequest{
				Username: test.username,
				Password: test.password,
			})

			requireCode(t, err, "auth.invalid_credentials")
			assert.Empty(t, credentials.calls)
		})
	}
}

func TestLoginPropagatesPortErrors(t *testing.T) {
	t.Run("credential store failure", func(t *testing.T) {
		storeErr := errors.New("identity unreachable")
		credentials := &stubCredentialStore{err: storeErr}
		tokens := &stubTokenIssuer{}
		sut := application.NewAuthService(credentials, tokens)

		_, err := sut.Login(context.Background(), application.LoginRequest{
			Username: "jrb",
			Password: "secret",
		})

		require.ErrorIs(t, err, storeErr)
		assert.Empty(t, tokens.createCalls)
	})

	t.Run("token issuer failure", func(t *testing.T) {
		tokenErr := errors.New("signing key unavailable")
		credentials := &stubCredentialStore{
			decision: domain.CredentialValidationResult{Valid: true},
		}
		tokens := &stubTokenIssuer{createErr: tokenErr}
		sut := application.NewAuthService(credentials, tokens)

		_, err := sut.Login(context.Background(), application.LoginRequest{
			Username: "jrb",
			Password: "secret",
		})

		require.ErrorIs(t, err, tokenErr)
	})
}

func TestLoginHonorsCancellationBeforeValidating(t *testing.T) {
	credentials := &stubCredentialStore{}
	tokens := &stubTokenIssuer{}
	sut := application.NewAuthService(credentials, tokens)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := sut.Login(ctx, application.LoginRequest{Username: "jrb", Password: "secret"})

	require.ErrorIs(t, err, context.Canceled)
	assert.Empty(t, credentials.calls)
}

func TestValidateDelegatesToTheTokenIssuer(t *testing.T) {
	tokens := &stubTokenIssuer{validateResult: domain.ValidateResult{Valid: true, Username: "jrb"}}
	sut := application.NewAuthService(&stubCredentialStore{}, tokens)

	result, err := sut.Validate(context.Background(), "token")

	require.NoError(t, err)
	assert.True(t, result.Valid)
	assert.Equal(t, "jrb", result.Username)
	assert.Equal(t, 1, tokens.validateCalls)
}

func TestValidatePropagatesANegativeResult(t *testing.T) {
	tokens := &stubTokenIssuer{validateResult: domain.ValidateResult{}}
	sut := application.NewAuthService(&stubCredentialStore{}, tokens)

	result, err := sut.Validate(context.Background(), "bad")

	require.NoError(t, err)
	assert.False(t, result.Valid)
	assert.Empty(t, result.Username)
}
