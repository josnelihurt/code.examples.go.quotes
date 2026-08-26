// Package application implements the auth use cases (login and token
// introspection) on top of the domain's credential and token ports. Login
// returns (LoginResult, error) where rejected credentials are a *domain.Error
// carrying the wire-visible auth.invalid_credentials code the transport maps
// to a 401; infrastructure failures surface as raw errors for the transport
// to map to a 500. Rate limiting is a transport concern and lives there.
package application

import (
	"context"
	"strings"

	"github.com/josnelihurt/code.examples.go.quotes/internal/auth/domain"
)

// LoginRequest is the raw input for the login use case.
type LoginRequest struct {
	Username string
	Password string
}

// LoginResult is a successful login: the minted token, the authenticated
// username, and the token's lifetime in seconds.
type LoginResult struct {
	AccessToken string
	Username    string
	ExpiresIn   int
}

// AuthService is the login/validate use case over the credential and token
// ports.
type AuthService struct {
	credentials domain.CredentialStore
	tokens      domain.TokenIssuer
}

// NewAuthService assembles the service on top of the credential store and the
// token issuer.
func NewAuthService(credentials domain.CredentialStore, tokens domain.TokenIssuer) *AuthService {
	return &AuthService{credentials: credentials, tokens: tokens}
}

// Login validates the credentials and mints a token. Blank input and rejected
// credentials both return auth.invalid_credentials — the same answer for an
// unknown user and a wrong password.
func (s *AuthService) Login(ctx context.Context, request LoginRequest) (LoginResult, error) {
	if err := ctx.Err(); err != nil {
		return LoginResult{}, err
	}

	if strings.TrimSpace(request.Username) == "" || strings.TrimSpace(request.Password) == "" {
		return LoginResult{}, domain.InvalidCredentials()
	}

	decision, err := s.credentials.Validate(ctx, request.Username, request.Password)
	if err != nil {
		return LoginResult{}, err
	}

	if !decision.Valid {
		return LoginResult{}, domain.InvalidCredentials()
	}

	issued, err := s.tokens.CreateToken(ctx, request.Username, decision.Scopes)
	if err != nil {
		return LoginResult{}, err
	}

	return LoginResult{
		AccessToken: issued.AccessToken,
		Username:    request.Username,
		ExpiresIn:   issued.ExpiresInSeconds,
	}, nil
}

// Validate introspects an access token. The answer (valid or not) is data,
// not an error — an invalid token returns a ValidateResult with Valid=false
// rather than an error.
func (s *AuthService) Validate(ctx context.Context, accessToken string) (domain.ValidateResult, error) {
	return s.tokens.ValidateToken(ctx, accessToken)
}
