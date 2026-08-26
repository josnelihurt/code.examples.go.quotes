package v3

import (
	"context"
	"net/http"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/josnelihurt/code.examples.go.quotes/internal/platform/problemjson"
)

// Scope vocabulary the v3 routes require (the .NET QuoteScopes port: reads
// need quotes:read, create needs quotes:write).
const (
	ScopeRead  = "quotes:read"
	ScopeWrite = "quotes:write"
)

// ErrorCodes and detail strings pinned with the .NET JwtAuthExtensions: the
// 401 problem body is byte-identical to every other transport's because both
// build it from the same shared problem factory.
const (
	TokenMissingErrorCode = "auth.token_missing"
	TokenInvalidErrorCode = "auth.token_invalid"
	tokenRequiredDetail   = "A valid bearer token is required."
)

// Authenticator is the port the HTTP and grpc guards consult — the
// composition root adapts the auth context's validator onto it (bounded
// contexts meet in composition, not by import). ok=false means the token was
// presented and rejected; a missing token never reaches the validator.
type Authenticator interface {
	// Authenticate returns the token's granted scopes when the token is valid.
	Authenticate(ctx context.Context, bearerToken string) (scopes []string, ok bool)
}

// routeScope is the method+path table the middleware enforces — the [Authorize]
// policies per rpc. Only routes the gateway owns carry a scope; anything else
// passes through for the gateway to answer.
func routeScope(method, path string) (string, bool) {
	switch {
	case method == http.MethodPost && path == "/api/v3/quotes":
		return ScopeWrite, true
	case method == http.MethodGet && strings.HasPrefix(path, "/api/v3/quotes"):
		return ScopeRead, true
	default:
		return "", false
	}
}

// methodScopes is the grpc-side table — the same policies keyed by full
// method name, enforced by ScopeUnaryInterceptor.
var methodScopes = map[string]string{
	"/quotes.v3.QuoteService/GetRandomQuote": ScopeRead,
	"/quotes.v3.QuoteService/ListQuotes":     ScopeRead,
	"/quotes.v3.QuoteService/GetQuoteById":   ScopeRead,
	"/quotes.v3.QuoteService/CreateQuote":    ScopeWrite,
}

// RequireScope is the authentication/authorization middleware in front of the
// gateway — how the .NET pipeline's byte-parity 401 and empty 403 are
// reproduced: they are answered here, BEFORE the gateway, so they never
// surface through DefaultHTTPErrorHandler (which would set WWW-Authenticate
// to the status message instead of the RFC 9110 challenge).
//
//   - Missing bearer token: 401 problem+json, errorCode auth.token_missing,
//     WWW-Authenticate: Bearer — the shared problem factory's exact shape.
//   - Presented but rejected: 401 problem+json, errorCode
//     auth.token_invalid, WWW-Authenticate: Bearer error="invalid_token".
//   - Valid token without the route's scope: 403 with an empty body — the
//     authorization middleware's own answer, pinned empty by the wire tests.
func RequireScope(auth Authenticator) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			scope, guarded := routeScope(r.Method, r.URL.Path)
			if !guarded {
				next.ServeHTTP(w, r)
				return
			}

			token, presented := parseBearer(r.Header.Get("Authorization"))
			if !presented {
				w.Header().Set("WWW-Authenticate", "Bearer")
				problemjson.Write(w, r, http.StatusUnauthorized, TokenMissingErrorCode, tokenRequiredDetail)
				return
			}

			scopes, ok := auth.Authenticate(r.Context(), token)
			if !ok {
				w.Header().Set("WWW-Authenticate", `Bearer error="invalid_token"`)
				problemjson.Write(w, r, http.StatusUnauthorized, TokenInvalidErrorCode, tokenRequiredDetail)
				return
			}

			if !containsScope(scopes, scope) {
				w.WriteHeader(http.StatusForbidden) // empty body, like the .NET middleware
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// ScopeUnaryInterceptor enforces the same method→scope table inside the grpc
// server — defense in depth for a caller that dials the grpc listener
// directly. It is unreachable through the HTTP path (the middleware above
// answers 401/403 first, and owns those wire shapes); a direct grpc caller
// gets the canonical codes instead: Unauthenticated / PermissionDenied.
func ScopeUnaryInterceptor(auth Authenticator) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		scope, guarded := methodScopes[info.FullMethod]
		if !guarded {
			return handler(ctx, req)
		}

		token := ""
		if md, ok := metadata.FromIncomingContext(ctx); ok {
			if values := md.Get("authorization"); len(values) > 0 {
				token = values[0]
			}
		}
		if candidate, presented := parseBearer(token); presented {
			token = candidate
		} else {
			token = ""
		}

		if token == "" {
			return nil, status.Error(codes.Unauthenticated, "A valid bearer token is required.")
		}

		scopes, ok := auth.Authenticate(ctx, token)
		if !ok {
			return nil, status.Error(codes.Unauthenticated, "A valid bearer token is required.")
		}
		if !containsScope(scopes, scope) {
			return nil, status.Error(codes.PermissionDenied, "The token does not grant the scope this method requires.")
		}

		return handler(ctx, req)
	}
}

// parseBearer extracts the token from an Authorization header, tolerating
// case and surrounding whitespace. An empty candidate ("Bearer ") is no token
// at all — the missing-token 401, not the invalid-token one.
func parseBearer(authorizationHeader string) (string, bool) {
	const prefix = "Bearer "
	if len(authorizationHeader) < len(prefix) ||
		!strings.EqualFold(authorizationHeader[:len(prefix)], prefix) {
		return "", false
	}
	candidate := strings.TrimSpace(authorizationHeader[len(prefix):])
	if candidate == "" {
		return "", false
	}
	return candidate, true
}

// containsScope reports whether the granted scopes carry the required one.
// The validator splits both claim forms (one space-separated string and
// repeated claims) before they get here.
func containsScope(scopes []string, required string) bool {
	for _, scope := range scopes {
		if scope == required {
			return true
		}
	}
	return false
}
