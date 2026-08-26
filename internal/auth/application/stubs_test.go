package application_test

import (
	"context"
	"testing"

	"github.com/josnelihurt/code.examples.go.quotes/internal/auth/domain"
	"github.com/stretchr/testify/require"
)

// stubCredentialCall records one Validate invocation.
type stubCredentialCall struct {
	username string
	password string
}

// stubCredentialStore is the hand-written double for the credential port.
type stubCredentialStore struct {
	decision domain.CredentialValidationResult
	err      error

	calls []stubCredentialCall
}

func (s *stubCredentialStore) Validate(_ context.Context, username, password string) (domain.CredentialValidationResult, error) {
	s.calls = append(s.calls, stubCredentialCall{username: username, password: password})
	return s.decision, s.err
}

// stubTokenCreateCall records one CreateToken invocation.
type stubTokenCreateCall struct {
	username string
	scopes   []string
}

// stubTokenIssuer is the hand-written double for the token port.
type stubTokenIssuer struct {
	issued         domain.IssuedToken
	createErr      error
	validateResult domain.ValidateResult
	validateErr    error

	createCalls   []stubTokenCreateCall
	validateCalls int
}

func (s *stubTokenIssuer) CreateToken(_ context.Context, username string, scopes []string) (domain.IssuedToken, error) {
	s.createCalls = append(s.createCalls, stubTokenCreateCall{username: username, scopes: scopes})
	return s.issued, s.createErr
}

func (s *stubTokenIssuer) ValidateToken(_ context.Context, _ string) (domain.ValidateResult, error) {
	s.validateCalls++
	return s.validateResult, s.validateErr
}

// requireCode asserts the error is a canonical domain error with the exact code.
func requireCode(t *testing.T, err error, code string) {
	t.Helper()
	var domainErr *domain.Error
	require.ErrorAs(t, err, &domainErr)
	require.Equal(t, code, domainErr.Code)
}
