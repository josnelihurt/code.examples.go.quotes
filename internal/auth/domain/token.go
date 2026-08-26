package domain

import "context"

// Scope vocabulary minted into tokens. The scope claim type and values are
// shared with the platform's JWT validation policies; the two sides are pinned
// together by a drift test once the transport layer lands.
const (
	ScopeClaimType   = "scope"
	ScopeQuotesRead  = "quotes:read"
	ScopeQuotesWrite = "quotes:write"
)

// IssuedToken is a freshly minted token together with its configured lifetime.
type IssuedToken struct {
	AccessToken      string
	ExpiresInSeconds int
}

// ValidateResult is the RFC 7662-style introspection answer: the verdict
// (valid or not) is data, not an error — an invalid token returns Valid=false
// rather than an error. Username is empty when the token is invalid.
type ValidateResult struct {
	Valid    bool
	Username string
}

// TokenIssuer is the port for minting and introspecting access tokens; the
// production adapter signs JWTs, tests mint opaque strings.
type TokenIssuer interface {
	// CreateToken mints an access token for the username carrying the granted
	// scopes.
	CreateToken(ctx context.Context, username string, scopes []string) (IssuedToken, error)

	// ValidateToken introspects an access token; an invalid token is a
	// ValidateResult with Valid=false, not an error.
	ValidateToken(ctx context.Context, accessToken string) (ValidateResult, error)
}
