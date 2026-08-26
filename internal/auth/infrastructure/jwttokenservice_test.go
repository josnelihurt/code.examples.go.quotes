package infrastructure_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/josnelihurt/code.examples.go.quotes/internal/auth/domain"
	"github.com/josnelihurt/code.examples.go.quotes/internal/auth/infrastructure"
	"github.com/josnelihurt/code.examples.go.quotes/internal/platform/config"
)

const testSigningKey = "unit-test-signing-key-that-is-long-enough-1234567890"

func newService(t *testing.T, cfg config.Jwt, environment string) *infrastructure.JwtTokenService {
	t.Helper()
	service, err := infrastructure.NewJwtTokenService(&cfg, environment, slog.New(slog.DiscardHandler))
	require.NoError(t, err)
	return service
}

func defaultCfg() config.Jwt {
	return config.Jwt{SigningKey: testSigningKey}
}

// payload decodes the token payload without validating it, so the issued
// claims are pinned independently of the validator under test.
func payload(t *testing.T, token string) map[string]any {
	t.Helper()
	parts := strings.Split(token, ".")
	require.Len(t, parts, 3, "a JWS compact token")

	decoded, err := base64.RawURLEncoding.DecodeString(parts[1])
	require.NoError(t, err)

	var claims map[string]any
	require.NoError(t, json.Unmarshal(decoded, &claims))
	return claims
}

func TestConstructionFailsOnAMissingSigningKey(t *testing.T) {
	_, err := infrastructure.NewJwtTokenService(&config.Jwt{}, config.EnvironmentDevelopment, slog.New(slog.DiscardHandler))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "required")
}

func TestConstructionRejectsAKeyBelowThePlatformMinimum(t *testing.T) {
	cfg := config.Jwt{SigningKey: strings.Repeat("k", config.MinimumSigningKeyBytes-1)}
	_, err := infrastructure.NewJwtTokenService(&cfg, config.EnvironmentDevelopment, slog.New(slog.DiscardHandler))
	require.Error(t, err)

	atFloor := config.Jwt{SigningKey: strings.Repeat("k", config.MinimumSigningKeyBytes)}
	_, err = infrastructure.NewJwtTokenService(&atFloor, config.EnvironmentDevelopment, slog.New(slog.DiscardHandler))
	require.NoError(t, err)
}

func TestConstructionRefusesTheDevelopmentKeyInProduction(t *testing.T) {
	cfg := config.Jwt{SigningKey: config.DevelopmentSigningKey}
	_, err := infrastructure.NewJwtTokenService(&cfg, config.EnvironmentProduction, slog.New(slog.DiscardHandler))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "Production")
}

func TestConstructionAcceptsTheDevelopmentKeyOutsideProduction(t *testing.T) {
	cfg := config.Jwt{SigningKey: config.DevelopmentSigningKey}
	_, err := infrastructure.NewJwtTokenService(&cfg, config.EnvironmentDevelopment, slog.New(slog.DiscardHandler))

	require.NoError(t, err)
}

func TestCreateTokenReportsTheConfiguredLifetime(t *testing.T) {
	cfg := defaultCfg()
	cfg.ExpiresInSeconds = 120
	sut := newService(t, cfg, config.EnvironmentDevelopment)

	issued, err := sut.CreateToken(context.Background(), "jrb", []string{domain.ScopeQuotesRead})
	require.NoError(t, err)

	assert.Equal(t, 120, issued.ExpiresInSeconds)

	claims := payload(t, issued.AccessToken)
	issuedAt := int64(claims["iat"].(float64))
	expiresAt := int64(claims["exp"].(float64))
	assert.Equal(t, int64(120), expiresAt-issuedAt)
}

func TestCreateTokenFallsBackToAnHourWhenTheLifetimeIsNotPositive(t *testing.T) {
	cfg := defaultCfg()
	cfg.ExpiresInSeconds = -1
	sut := newService(t, cfg, config.EnvironmentDevelopment)

	issued, err := sut.CreateToken(context.Background(), "jrb", []string{domain.ScopeQuotesRead})
	require.NoError(t, err)

	assert.Equal(t, 3600, issued.ExpiresInSeconds)
}

func TestTheIssuedClaimsRoundTrip(t *testing.T) {
	sut := newService(t, defaultCfg(), config.EnvironmentDevelopment)

	issued, err := sut.CreateToken(context.Background(), "jrb",
		[]string{domain.ScopeQuotesWrite, domain.ScopeQuotesRead, domain.ScopeQuotesRead})
	require.NoError(t, err)

	claims := payload(t, issued.AccessToken)
	assert.Equal(t, "auth-api", claims["iss"])
	audience, ok := claims["aud"].([]any)
	require.True(t, ok, "the audience claim is an array")
	require.Len(t, audience, 1)
	assert.Equal(t, "aspire-quotes-poc", audience[0])
	assert.Equal(t, "jrb", claims["sub"])
	assert.Equal(t, "jrb", claims["name"])
	assert.Equal(t, "quotes:read quotes:write", claims["scope"], "scopes are deduplicated, sorted, space-joined")
	assert.Contains(t, claims, "iat")
	assert.Contains(t, claims, "exp")
}

func TestAFreshlyIssuedTokenValidatesAndCarriesTheUsername(t *testing.T) {
	sut := newService(t, defaultCfg(), config.EnvironmentDevelopment)

	issued, err := sut.CreateToken(context.Background(), "jrb", []string{domain.ScopeQuotesRead})
	require.NoError(t, err)

	result, err := sut.ValidateToken(context.Background(), issued.AccessToken)
	require.NoError(t, err)

	assert.True(t, result.Valid)
	assert.Equal(t, "jrb", result.Username)
}

func TestValidateAnswersInvalidForBlankGarbageAndForeignTokens(t *testing.T) {
	sut := newService(t, defaultCfg(), config.EnvironmentDevelopment)

	foreign := newService(t, config.Jwt{SigningKey: strings.Repeat("f", 32)}, config.EnvironmentDevelopment)
	foreignToken, err := foreign.CreateToken(context.Background(), "jrb", nil)
	require.NoError(t, err)

	for _, token := range []string{"", "   ", "not-a-jwt", foreignToken.AccessToken} {
		result, err := sut.ValidateToken(context.Background(), token)
		require.NoError(t, err)
		assert.False(t, result.Valid, token)
		assert.Empty(t, result.Username, token)
	}
}

func TestValidateToleratesOneMinuteOfSkewButNoMore(t *testing.T) {
	sut := newService(t, defaultCfg(), config.EnvironmentDevelopment)
	key := []byte(testSigningKey)
	now := time.Now()

	// Within the leeway window: 30 seconds stale still validates.
	freshEnough, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
		Issuer:    infrastructure.DefaultIssuer,
		Audience:  jwt.ClaimStrings{infrastructure.DefaultAudience},
		Subject:   "jrb",
		ExpiresAt: jwt.NewNumericDate(now.Add(-30 * time.Second)),
	}).SignedString(key)
	require.NoError(t, err)

	result, err := sut.ValidateToken(context.Background(), freshEnough)
	require.NoError(t, err)
	assert.True(t, result.Valid, "30 seconds of skew is inside the 1-minute leeway")

	// Beyond it: 2 minutes stale is rejected.
	stale, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
		Issuer:    infrastructure.DefaultIssuer,
		Audience:  jwt.ClaimStrings{infrastructure.DefaultAudience},
		Subject:   "jrb",
		ExpiresAt: jwt.NewNumericDate(now.Add(-2 * time.Minute)),
	}).SignedString(key)
	require.NoError(t, err)

	result, err = sut.ValidateToken(context.Background(), stale)
	require.NoError(t, err)
	assert.False(t, result.Valid, "2 minutes of skew is outside the leeway")
}

func TestValidateRejectsForeignIssuersAndAudiences(t *testing.T) {
	sut := newService(t, defaultCfg(), config.EnvironmentDevelopment)
	key := []byte(testSigningKey)
	now := time.Now()

	mint := func(issuer, audience string) string {
		token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
			Issuer:    issuer,
			Audience:  jwt.ClaimStrings{audience},
			Subject:   "jrb",
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour)),
		}).SignedString(key)
		require.NoError(t, err)
		return token
	}

	for _, token := range []string{
		mint("someone-else", infrastructure.DefaultAudience),
		mint(infrastructure.DefaultIssuer, "someone-else"),
	} {
		result, err := sut.ValidateToken(context.Background(), token)
		require.NoError(t, err)
		assert.False(t, result.Valid)
	}
}

func TestValidateRejectsTokensSignedWithAnotherAlgorithm(t *testing.T) {
	sut := newService(t, defaultCfg(), config.EnvironmentDevelopment)
	key := []byte(testSigningKey)

	confused, err := jwt.NewWithClaims(jwt.SigningMethodHS512, jwt.RegisteredClaims{
		Issuer:    infrastructure.DefaultIssuer,
		Audience:  jwt.ClaimStrings{infrastructure.DefaultAudience},
		Subject:   "jrb",
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
	}).SignedString(key)
	require.NoError(t, err)

	result, err := sut.ValidateToken(context.Background(), confused)
	require.NoError(t, err)
	assert.False(t, result.Valid, "only HS256 is a valid method")
}

func TestValidateRequiresAnExpiration(t *testing.T) {
	sut := newService(t, defaultCfg(), config.EnvironmentDevelopment)
	key := []byte(testSigningKey)

	nonExpiring, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
		Issuer:   infrastructure.DefaultIssuer,
		Audience: jwt.ClaimStrings{infrastructure.DefaultAudience},
		Subject:  "jrb",
	}).SignedString(key)
	require.NoError(t, err)

	result, err := sut.ValidateToken(context.Background(), nonExpiring)
	require.NoError(t, err)
	assert.False(t, result.Valid)
}
