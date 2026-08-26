package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/josnelihurt/code.examples.go.quotes/internal/platform/config"
)

// withEnv sets the canonical `__`-separated env form for the duration of the
// test (the compose/Kubernetes spelling).
func withEnv(t *testing.T, values map[string]string) {
	t.Helper()
	for key, value := range values {
		t.Setenv(key, value)
	}
}

func TestLoadFillsTheDefaults(t *testing.T) {
	withEnv(t, map[string]string{"JWT__SIGNINGKEY": strings.Repeat("k", 32)})

	cfg, err := config.Load("")
	require.NoError(t, err)

	assert.Equal(t, "localhost:8090", cfg.Server.Address)
	assert.Equal(t, config.EnvironmentDevelopment, cfg.Environment)
	assert.Equal(t, "auth-api", cfg.Jwt.Issuer)
	assert.Equal(t, "aspire-quotes-poc", cfg.Jwt.Audience)
	assert.Equal(t, 3600, cfg.Jwt.ExpiresInSeconds)
	assert.Equal(t, 10, cfg.RateLimiting.Auth.PermitLimit)
	assert.Equal(t, 30, cfg.RateLimiting.Auth.WindowSeconds)
}

func TestLoadBindsTheCanonicalDoubleUnderscoreEnvNames(t *testing.T) {
	withEnv(t, map[string]string{
		"JWT__SIGNINGKEY":                   strings.Repeat("k", 32),
		"JWT__ISSUER":                       "custom-issuer",
		"JWT__AUDIENCE":                     "custom-audience",
		"JWT__EXPIRESINSECONDS":             "120",
		"SERVER__ADDRESS":                   "localhost:9999",
		"ENVIRONMENT":                       "Production",
		"RATELIMITING__AUTH__PERMITLIMIT":   "3",
		"RATELIMITING__AUTH__WINDOWSECONDS": "45",
	})

	cfg, err := config.Load("")
	require.NoError(t, err)

	assert.Equal(t, strings.Repeat("k", 32), cfg.Jwt.SigningKey)
	assert.Equal(t, "custom-issuer", cfg.Jwt.Issuer)
	assert.Equal(t, "custom-audience", cfg.Jwt.Audience)
	assert.Equal(t, 120, cfg.Jwt.ExpiresInSeconds)
	assert.Equal(t, "localhost:9999", cfg.Server.Address)
	assert.Equal(t, "Production", cfg.Environment)
	assert.Equal(t, 3, cfg.RateLimiting.Auth.PermitLimit)
	assert.Equal(t, 45, cfg.RateLimiting.Auth.WindowSeconds)
}

func TestLoadAcceptsTheFlatSingleUnderscoreAlias(t *testing.T) {
	withEnv(t, map[string]string{
		"JWT_SIGNINGKEY": strings.Repeat("k", 32),
	})

	cfg, err := config.Load("")
	require.NoError(t, err)

	assert.Equal(t, strings.Repeat("k", 32), cfg.Jwt.SigningKey)
}

func TestLoadReadsTheOptionalConfigFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	require.NoError(t, os.WriteFile(path, []byte(`{
		"jwt": {"signingkey": "from-the-file-signing-key-long-enough", "issuer": "from-file"}
	}`), 0o600))

	cfg, err := config.Load(path)
	require.NoError(t, err)

	assert.Equal(t, "from-the-file-signing-key-long-enough", cfg.Jwt.SigningKey)
	assert.Equal(t, "from-file", cfg.Jwt.Issuer)
}

func TestValidateFailsFastOnAMissingSigningKey(t *testing.T) {
	_, err := config.Load("")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "jwt.signingkey")
	assert.Contains(t, err.Error(), "required")
}

func TestValidateFailsFastOnAShortSigningKey(t *testing.T) {
	withEnv(t, map[string]string{"JWT__SIGNINGKEY": strings.Repeat("k", config.MinimumSigningKeyBytes-1)})

	_, err := config.Load("")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be at least 32 bytes")
}

func TestValidateAcceptsAKeyAtTheMinimumLength(t *testing.T) {
	withEnv(t, map[string]string{"JWT__SIGNINGKEY": strings.Repeat("k", config.MinimumSigningKeyBytes)})

	_, err := config.Load("")

	require.NoError(t, err)
}

func TestValidateRefusesTheDevelopmentKeyInProduction(t *testing.T) {
	withEnv(t, map[string]string{
		"JWT__SIGNINGKEY": config.DevelopmentSigningKey,
		"ENVIRONMENT":     "Production",
	})

	_, err := config.Load("")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "public development key")
}

func TestValidateAllowsTheDevelopmentKeyOutsideProduction(t *testing.T) {
	withEnv(t, map[string]string{"JWT__SIGNINGKEY": config.DevelopmentSigningKey})

	_, err := config.Load("")

	require.NoError(t, err)
}

func TestValidateFailsFastOnANonPositivePermitLimit(t *testing.T) {
	withEnv(t, map[string]string{
		"JWT__SIGNINGKEY":                 strings.Repeat("k", 32),
		"RATELIMITING__AUTH__PERMITLIMIT": "0",
	})

	_, err := config.Load("")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "permitlimit")
}

func TestValidateFailsFastOnANonPositiveWindow(t *testing.T) {
	withEnv(t, map[string]string{
		"JWT__SIGNINGKEY":                   strings.Repeat("k", 32),
		"RATELIMITING__AUTH__WINDOWSECONDS": "0",
	})

	_, err := config.Load("")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "windowseconds")
}
