// Package api is the auth bounded context's HTTP transport: the login and
// token-introspection endpoints with the .NET Auth API's exact wire shapes
// (camelCase bodies, RFC 9457 problems with errorCode/correlationId
// extensions, per-IP fixed-window rate limiting on both routes).
package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"

	"github.com/josnelihurt/code.examples.go.quotes/internal/auth/application"
	"github.com/josnelihurt/code.examples.go.quotes/internal/auth/domain"
	"github.com/josnelihurt/code.examples.go.quotes/internal/platform/correlation"
	"github.com/josnelihurt/code.examples.go.quotes/internal/platform/problemjson"
	"github.com/josnelihurt/code.examples.go.quotes/internal/platform/telemetry"
)

// Endpoint routes.
const (
	LoginRoute    = "/api/v1/auth/login"
	ValidateRoute = "/api/v1/auth/validate"
)

// errorCode vocabulary owned by the transport layer (auth.invalid_credentials
// lives in the domain; validation.request_invalid in problemjson).
const (
	RateLimitedErrorCode  = "auth.rate_limited"
	TokenMissingErrorCode = "auth.token_missing"
	UnexpectedErrorCode   = "error.unknown"
)

// Detail strings pinned with the .NET kit.
const (
	rateLimitedDetail    = "The auth endpoint rate limit was exceeded; retry after the window elapses."
	tokenMissingDetail   = "An access token is required."
	unreadableBodyDetail = "The request body could not be read as JSON."
	unexpectedDetail     = "An unexpected error occurred."
)

// Field-rule bounds mirrored from the .NET request DTOs.
const (
	maxUsernameLength    = 100
	maxPasswordLength    = 200
	maxAccessTokenLength = 4096
)

// Limiter is the transport's rate-limit port; the infrastructure fixed-window
// limiter satisfies it (bounded contexts meet through ports, not imports).
type Limiter interface {
	Allow(clientKey string) bool
}

// loginRequest is the login body: {"username": "...", "password": "..."}.
type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// loginResponse mirrors LoginResponseDto field-for-field, order included:
// accessToken, correlationId, expiresIn, username.
type loginResponse struct {
	AccessToken   string `json:"accessToken"`
	CorrelationID string `json:"correlationId"`
	ExpiresIn     int    `json:"expiresIn"`
	Username      string `json:"username"`
}

// validateRequest is the optional validate body; the Authorization bearer
// header is the fallback.
type validateRequest struct {
	AccessToken string `json:"accessToken"`
}

// validateResponse mirrors ValidateResponseDto: valid is always present,
// username is null when the token is invalid (the .NET serializer writes the
// null rather than omitting the member).
type validateResponse struct {
	Valid    bool    `json:"valid"`
	Username *string `json:"username"`
}

// AuthAPI serves the auth endpoints over the layer-4 application service.
type AuthAPI struct {
	service *application.AuthService
	limiter Limiter
	metrics *telemetry.Metrics
	logger  *slog.Logger
}

// New composes the transport. The limiter guards both routes; the metrics
// record every login attempt and token introspection with the auth outcome
// vocabulary (success|failure).
func New(service *application.AuthService, limiter Limiter, metrics *telemetry.Metrics, logger *slog.Logger) *AuthAPI {
	return &AuthAPI{service: service, limiter: limiter, metrics: metrics, logger: logger}
}

// Register wires the endpoints on the mux.
func (a *AuthAPI) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST "+LoginRoute, a.rateLimited(a.handleLogin))
	mux.HandleFunc("POST "+ValidateRoute, a.rateLimited(a.handleValidate))
}

// rateLimited rejects requests over the client's window budget with the 429
// problem (no Retry-After — the .NET OnRejected handler sends none either).
func (a *AuthAPI) rateLimited(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !a.limiter.Allow(clientKey(r)) {
			problemjson.Write(w, r, http.StatusTooManyRequests, RateLimitedErrorCode, rateLimitedDetail)
			return
		}
		next(w, r)
	}
}

func (a *AuthAPI) handleLogin(w http.ResponseWriter, r *http.Request) {
	var body loginRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		problemjson.Write(w, r, http.StatusBadRequest, problemjson.RequestValidationErrorCode, unreadableBodyDetail)
		return
	}

	if fieldErrors := validateLogin(body); len(fieldErrors) > 0 {
		problemjson.WriteValidation(w, r, http.StatusBadRequest, problemjson.RequestValidationErrorCode, fieldErrors)
		return
	}

	result, err := a.service.Login(r.Context(), application.LoginRequest{
		Username: body.Username,
		Password: body.Password,
	})
	if err != nil {
		a.metrics.RecordLogin(r.Context(), telemetry.OutcomeFailure)

		var domainErr *domain.Error
		if errors.As(err, &domainErr) {
			problemjson.Write(w, r, statusFor(domainErr.Code), domainErr.Code, domainErr.Description)
			return
		}
		a.logger.Error("login failed", "error", err)
		problemjson.Write(w, r, http.StatusInternalServerError, UnexpectedErrorCode, unexpectedDetail)
		return
	}

	a.metrics.RecordLogin(r.Context(), telemetry.OutcomeSuccess)
	writeJSON(w, http.StatusOK, loginResponse{
		AccessToken:   result.AccessToken,
		CorrelationID: correlation.FromContext(r.Context()),
		ExpiresIn:     result.ExpiresIn,
		Username:      result.Username,
	})
}

func validateLogin(body loginRequest) map[string][]string {
	var errorsByField map[string][]string

	if strings.TrimSpace(body.Username) == "" {
		errorsByField = addError(errorsByField, "Username", "The Username field is required.")
	} else if len(body.Username) > maxUsernameLength {
		errorsByField = addError(errorsByField, "Username",
			"The field Username must be a string or array type with a maximum length of '"+strconv.Itoa(maxUsernameLength)+"'.")
	}

	if strings.TrimSpace(body.Password) == "" {
		errorsByField = addError(errorsByField, "Password", "The Password field is required.")
	} else if len(body.Password) > maxPasswordLength {
		errorsByField = addError(errorsByField, "Password",
			"The field Password must be a string or array type with a maximum length of '"+strconv.Itoa(maxPasswordLength)+"'.")
	}

	return errorsByField
}

func (a *AuthAPI) handleValidate(w http.ResponseWriter, r *http.Request) {
	var body *validateRequest
	if r.Body != nil && r.ContentLength != 0 {
		body = &validateRequest{}
		if err := json.NewDecoder(r.Body).Decode(body); err != nil {
			problemjson.Write(w, r, http.StatusBadRequest, problemjson.RequestValidationErrorCode, unreadableBodyDetail)
			return
		}
	}

	token := ""
	if body != nil {
		token = strings.TrimSpace(body.AccessToken)
	}
	if token == "" {
		if bearer, ok := parseBearer(r.Header.Get("Authorization")); ok {
			token = bearer
		}
	}

	if token == "" {
		// Bearer parsing is an API concern, so this pre-service failure cannot
		// move into the service: record it here, before the service is involved.
		a.metrics.RecordValidate(r.Context(), telemetry.OutcomeFailure)
		a.logger.Warn("Token validation request carried no token")
		problemjson.WriteValidation(w, r, http.StatusBadRequest, TokenMissingErrorCode,
			map[string][]string{TokenMissingErrorCode: {tokenMissingDetail}})
		return
	}

	if len(token) > maxAccessTokenLength {
		problemjson.WriteValidation(w, r, http.StatusBadRequest, problemjson.RequestValidationErrorCode,
			map[string][]string{"AccessToken": {
				"The field AccessToken must be a string or array type with a maximum length of '" + strconv.Itoa(maxAccessTokenLength) + "'.",
			}})
		return
	}

	result, err := a.service.Validate(r.Context(), token)
	if err != nil {
		a.metrics.RecordValidate(r.Context(), telemetry.OutcomeFailure)
		a.logger.Error("token validation failed", "error", err)
		problemjson.Write(w, r, http.StatusInternalServerError, UnexpectedErrorCode, unexpectedDetail)
		return
	}

	if result.Valid {
		username := result.Username
		a.metrics.RecordValidate(r.Context(), telemetry.OutcomeSuccess)
		writeJSON(w, http.StatusOK, validateResponse{Valid: true, Username: &username})
		return
	}

	a.metrics.RecordValidate(r.Context(), telemetry.OutcomeFailure)
	writeJSON(w, http.StatusOK, validateResponse{Valid: false, Username: nil})
}

// statusFor maps a domain error code onto its HTTP status — the transport
// half of the ErrorOr StatusCode table for the codes this context mints.
func statusFor(code string) int {
	if code == domain.InvalidCredentials().Code {
		return http.StatusUnauthorized
	}
	return http.StatusInternalServerError
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// parseBearer extracts the token from an Authorization header, tolerating
// case and surrounding whitespace — the BearerToken.TryParse port. An empty
// candidate ("Bearer ") is no token at all.
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

// clientKey partitions the limiter by remote IP (the .NET
// Connection.RemoteIpAddress partition), with the same unknown-client
// fallback for address-less requests.
func clientKey(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	if host == "" {
		return "unknown-client"
	}
	return host
}

func addError(errorsByField map[string][]string, field, message string) map[string][]string {
	if errorsByField == nil {
		errorsByField = make(map[string][]string, 2)
	}
	errorsByField[field] = append(errorsByField[field], message)
	return errorsByField
}
