// Package infrastructure holds the auth context's adapters: the HS256 JWT
// token service, the hardcoded development credential store, and the
// fixed-window rate limiter backing the transport's 429s.
package infrastructure

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/josnelihurt/code.examples.go.quotes/internal/auth/domain"
	"github.com/josnelihurt/code.examples.go.quotes/internal/platform/config"
)

// DefaultIssuer and DefaultAudience are wire contract: the frontend and the
// resource APIs validate tokens against these exact values. They mirror the
// .NET kit's JwtAuthExtensions defaults; drift breaks token parity.
const (
	DefaultIssuer   = "auth-api"
	DefaultAudience = "aspire-quotes-poc"
)

// tokenLeeway is the clock skew tolerated on validation (the .NET kit's
// 1-minute ClockSkew parity).
const tokenLeeway = time.Minute

// claims is the issued token payload: registered claims (iss/aud/sub/iat/exp)
// plus the name and scope members. The scope claim is space-separated per
// RFC 8693 (one claim carrying every granted value) on the wire we mint, but
// scopeClaims also parses the repeated-claims form (a JSON array) some
// issuers emit — .NET's claim mapping accepts both, so validation does too.
type claims struct {
	jwt.RegisteredClaims
	Name  string      `json:"name"`
	Scope scopeClaims `json:"scope"`
}

// scopeClaims is the scope claim in both forms it legally travels: one
// space-separated string, or repeated claims (a JSON array). Unmarshaling
// splits either into values; marshaling writes the single-string form so
// tokens minted here keep their exact historical shape.
type scopeClaims []string

// UnmarshalJSON accepts the string form and the repeated-claims form (a JSON
// array), splitting on whitespace inside either and merging everything into
// plain values. Anything else is a malformed scope claim — an error that
// fails the whole validation.
func (s *scopeClaims) UnmarshalJSON(data []byte) error {
	var single string
	if err := json.Unmarshal(data, &single); err == nil {
		*s = strings.Fields(single)
		return nil
	}

	var repeated []string
	if err := json.Unmarshal(data, &repeated); err == nil {
		values := make([]string, 0, len(repeated))
		for _, entry := range repeated {
			values = append(values, strings.Fields(entry)...)
		}
		*s = values
		return nil
	}

	return errors.New("the scope claim must be a string or an array of strings")
}

// MarshalJSON writes the single space-separated claim (RFC 8693) — the exact
// bytes CreateToken has always minted.
func (s scopeClaims) MarshalJSON() ([]byte, error) {
	return json.Marshal(strings.Join(s, " "))
}

// JwtTokenService issues and introspects HS256 access tokens — the .NET
// JwtTokenService port on the layer-4 TokenIssuer port. Misconfiguration
// fails at construction (boot), never at first use.
type JwtTokenService struct {
	signingKey       []byte
	issuer           string
	audience         string
	expiresInSeconds int
	logger           *slog.Logger
}

// NewJwtTokenService validates the configuration and builds the service.
// Boot guards, verbatim from the .NET kit: a missing signing key is fatal, a
// key under config.MinimumSigningKeyBytes is fatal (HMAC-SHA256 with a short
// key is a misconfiguration, not a degraded mode), and the public development
// key is fatal in Production.
func NewJwtTokenService(cfg *config.Jwt, environment string, logger *slog.Logger) (*JwtTokenService, error) {
	if cfg.SigningKey == "" {
		return nil, errors.New("jwt.signingkey (Jwt:SigningKey) is required")
	}
	if len(cfg.SigningKey) < config.MinimumSigningKeyBytes {
		return nil, fmt.Errorf(
			"jwt.signingkey (Jwt:SigningKey) must be at least %d bytes",
			config.MinimumSigningKeyBytes)
	}
	if environment == config.EnvironmentProduction && cfg.SigningKey == config.DevelopmentSigningKey {
		return nil, errors.New(
			"jwt.signingkey (Jwt:SigningKey) is set to the public development key; configure a real secret before running in Production")
	}

	expiresInSeconds := cfg.ExpiresInSeconds
	if expiresInSeconds <= 0 {
		expiresInSeconds = 3600
	}

	return &JwtTokenService{
		signingKey:       []byte(cfg.SigningKey),
		issuer:           orDefault(cfg.Issuer, DefaultIssuer),
		audience:         orDefault(cfg.Audience, DefaultAudience),
		expiresInSeconds: expiresInSeconds,
		logger:           logger,
	}, nil
}

func orDefault(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

// CreateToken mints an access token for the username carrying the granted
// scopes (de-duplicated, order-stable).
func (s *JwtTokenService) CreateToken(_ context.Context, username string, scopes []string) (domain.IssuedToken, error) {
	now := time.Now()
	payload := claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    s.issuer,
			Audience:  jwt.ClaimStrings{s.audience},
			Subject:   username,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Duration(s.expiresInSeconds) * time.Second)),
		},
		Name:  username,
		Scope: normalizeScopes(scopes),
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, payload).SignedString(s.signingKey)
	if err != nil {
		return domain.IssuedToken{}, fmt.Errorf("signing the access token: %w", err)
	}

	return domain.IssuedToken{AccessToken: token, ExpiresInSeconds: s.expiresInSeconds}, nil
}

func normalizeScopes(scopes []string) scopeClaims {
	seen := make(map[string]struct{}, len(scopes))
	unique := make([]string, 0, len(scopes))
	for _, scope := range scopes {
		if _, dup := seen[scope]; dup {
			continue
		}
		seen[scope] = struct{}{}
		unique = append(unique, scope)
	}
	sort.Strings(unique)
	return scopeClaims(unique)
}

// ValidateToken introspects an access token. An invalid token is data, not an
// error: ValidateResult{Valid: false}. Blank tokens are invalid; validation
// failures log at Warn and answer invalid — indistinguishable from the .NET
// catch-all. Validation requires the issuer and audience, an expiration, the
// HS256 alg (blocking alg confusion), and tolerates one minute of skew.
func (s *JwtTokenService) ValidateToken(_ context.Context, accessToken string) (domain.ValidateResult, error) {
	if strings.TrimSpace(accessToken) == "" {
		return domain.ValidateResult{}, nil
	}

	parsed, err := jwt.ParseWithClaims(accessToken, &claims{},
		func(*jwt.Token) (any, error) { return s.signingKey, nil },
		jwt.WithIssuer(s.issuer),
		jwt.WithAudience(s.audience),
		jwt.WithLeeway(tokenLeeway),
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithExpirationRequired(),
	)
	if err != nil || !parsed.Valid {
		s.logger.Warn("JWT validation failed", "error", err)
		return domain.ValidateResult{}, nil
	}

	payload, ok := parsed.Claims.(*claims)
	if !ok {
		return domain.ValidateResult{}, nil
	}

	username := payload.Name
	if username == "" {
		username = payload.Subject
	}
	if username == "" {
		return domain.ValidateResult{}, nil
	}

	return domain.ValidateResult{
		Valid:    true,
		Username: username,
		Scopes:   payload.Scope,
	}, nil
}
