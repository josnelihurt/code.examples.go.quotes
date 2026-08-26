// Package config loads the application configuration with Viper following
// ADR 0004: defaults layer -> optional config file -> environment variables,
// with `__` as the section separator so the same compose/Kubernetes manifests
// that configure the .NET kit (`Jwt__SigningKey`,
// `ConnectionStrings__quotesdb`, `RateLimiting__Auth__PermitLimit`) configure
// this port byte-for-byte. The Viper instance is constructed here and
// discarded — the immutable *Config is what gets injected; misconfiguration
// fails at boot via Validate, never at first use.
package config

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

// DevelopmentSigningKey is the documented development signing key (the .NET
// kit's user-secrets value). Production startup fails if it is ever the
// configured key — it is public knowledge and signs nothing worth trusting.
const DevelopmentSigningKey = "AspireQuotesPoc-Dev-Signing-Key-32chars!"

// MinimumSigningKeyBytes is the least HMAC-SHA256 key length accepted at boot.
const MinimumSigningKeyBytes = 32

// Environment names (the .NET Environments.* vocabulary).
const (
	EnvironmentDevelopment = "Development"
	EnvironmentProduction  = "Production"
)

// Server is the HTTP listener configuration.
type Server struct {
	Address string `mapstructure:"address"`
}

// Jwt is the token-signing configuration. Issuer and audience defaults are
// wire contract: the frontend and resource APIs validate tokens against these
// exact values.
type Jwt struct {
	SigningKey       string `mapstructure:"signingkey"`
	Issuer           string `mapstructure:"issuer"`
	Audience         string `mapstructure:"audience"`
	ExpiresInSeconds int    `mapstructure:"expiresinseconds"`
}

// ConnectionStrings holds the database connection URIs by name.
type ConnectionStrings struct {
	QuotesDb string `mapstructure:"quotesdb"`
}

// AuthRateLimit is the fixed-window limiter settings for the auth endpoints.
type AuthRateLimit struct {
	PermitLimit   int `mapstructure:"permitlimit"`
	WindowSeconds int `mapstructure:"windowseconds"`
}

// RateLimiting groups the per-area limiter settings.
type RateLimiting struct {
	Auth AuthRateLimit `mapstructure:"auth"`
}

// Config is the fully-resolved, immutable application configuration. Services
// that own a database additionally fail fast on a missing
// ConnectionStrings.QuotesDb at their composition root — the auth API owns
// none, so Validate leaves it unchecked here.
type Config struct {
	Server            Server            `mapstructure:"server"`
	Environment       string            `mapstructure:"environment"`
	Jwt               Jwt               `mapstructure:"jwt"`
	ConnectionStrings ConnectionStrings `mapstructure:"connectionstrings"`
	RateLimiting      RateLimiting      `mapstructure:"ratelimiting"`
}

// flatEnvAliases maps canonical config keys to the single-underscore flat env
// form (`JWT_SIGNINGKEY`) declared alongside the struct per ADR 0004; the
// canonical form is always the `__`-separated one (`JWT__SIGNINGKEY`).
var flatEnvAliases = map[string]string{
	"server.address":                  "SERVER_ADDRESS",
	"environment":                     "ENVIRONMENT",
	"jwt.signingkey":                  "JWT_SIGNINGKEY",
	"jwt.issuer":                      "JWT_ISSUER",
	"jwt.audience":                    "JWT_AUDIENCE",
	"jwt.expiresinseconds":            "JWT_EXPIRESINSECONDS",
	"connectionstrings.quotesdb":      "CONNECTIONSTRINGS_QUOTESDB",
	"ratelimiting.auth.permitlimit":   "RATELIMITING_AUTH_PERMITLIMIT",
	"ratelimiting.auth.windowseconds": "RATELIMITING_AUTH_WINDOWSECONDS",
}

// Load resolves the configuration: defaults for every platform key, then the
// optional file at path (an empty path searches the working directory; an
// absent file is not an error), then environment variables. Callers must run
// Validate before serving.
func Load(path string) (*Config, error) {
	v := viper.New()

	setDefaults(v)

	if path != "" {
		v.SetConfigFile(path)
	} else {
		v.AddConfigPath(".")
		v.SetConfigName("config")
	}
	_ = v.ReadInConfig() // an absent or unreadable file falls through to env

	v.SetEnvKeyReplacer(strings.NewReplacer(".", "__"))
	v.AutomaticEnv()
	for key, alias := range flatEnvAliases {
		_ = v.BindEnv(key, alias)
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("decoding the configuration: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func setDefaults(v *viper.Viper) {
	// Every platform key needs a default even when the default is empty:
	// Viper's Unmarshal walks only AllKeys() (defaults + file keys), so an
	// env-only override silently misses the struct without one (ADR 0004).
	v.SetDefault("server.address", "localhost:8090")
	v.SetDefault("environment", EnvironmentDevelopment)
	v.SetDefault("jwt.signingkey", "")
	v.SetDefault("jwt.issuer", "auth-api")
	v.SetDefault("jwt.audience", "aspire-quotes-poc")
	v.SetDefault("jwt.expiresinseconds", 3600)
	v.SetDefault("connectionstrings.quotesdb", "")
	v.SetDefault("ratelimiting.auth.permitlimit", 10)
	v.SetDefault("ratelimiting.auth.windowseconds", 30)
}

// Validate is the fail-fast boot guard (the .NET kit's InvalidOperationException
// stance): a missing or short signing key, the public dev key in Production,
// and a non-positive rate-limit permit budget are each fatal before serving.
func (c *Config) Validate() error {
	var problems []error

	key := c.Jwt.SigningKey
	if key == "" {
		problems = append(problems, errors.New("jwt.signingkey (Jwt:SigningKey) is required"))
	} else if len(key) < MinimumSigningKeyBytes {
		problems = append(problems, fmt.Errorf(
			"jwt.signingkey (Jwt:SigningKey) must be at least %d bytes; configure a real secret",
			MinimumSigningKeyBytes))
	}

	if c.IsProduction() && key == DevelopmentSigningKey {
		problems = append(problems, errors.New(
			"jwt.signingkey (Jwt:SigningKey) is set to the public development key; configure a real secret before running in Production"))
	}

	if c.RateLimiting.Auth.PermitLimit <= 0 {
		problems = append(problems, errors.New(
			"ratelimiting.auth.permitlimit (RateLimiting:Auth:PermitLimit) must be positive"))
	}
	if c.RateLimiting.Auth.WindowSeconds <= 0 {
		problems = append(problems, errors.New(
			"ratelimiting.auth.windowseconds (RateLimiting:Auth:WindowSeconds) must be positive"))
	}

	return errors.Join(problems...)
}

// IsProduction reports whether the host runs in the Production environment.
func (c *Config) IsProduction() bool {
	return c.Environment == EnvironmentProduction
}
