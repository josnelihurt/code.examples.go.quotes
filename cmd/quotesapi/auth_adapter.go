package main

import (
	"context"

	"github.com/josnelihurt/code.examples.go.quotes/internal/auth/domain"
)

// tokenValidator is the auth context's validator port as this composition
// sees it — the JwtTokenService satisfies it structurally.
type tokenValidator interface {
	ValidateToken(ctx context.Context, accessToken string) (domain.ValidateResult, error)
}

// bearerAuthenticator adapts the auth context's validator onto the v3
// transport's Authenticator port: the composition root is where the two
// bounded contexts meet (the layering guard enforces exactly that). A
// validation failure is not an error here — ok=false is the transport's
// invalid-token 401.
type bearerAuthenticator struct {
	validator tokenValidator
}

// Authenticate returns the token's granted scopes when it is valid.
func (a bearerAuthenticator) Authenticate(ctx context.Context, bearerToken string) ([]string, bool) {
	result, err := a.validator.ValidateToken(ctx, bearerToken)
	if err != nil || !result.Valid {
		return nil, false
	}
	return result.Scopes, true
}
