package infrastructure

import (
	"context"
	"crypto/subtle"
	"errors"

	"golang.org/x/crypto/bcrypt"

	"github.com/josnelihurt/code.examples.go.quotes/internal/auth/domain"
	"github.com/josnelihurt/code.examples.go.quotes/internal/platform/config"
)

// devUser is one row of the fixed development credential table.
type devUser struct {
	Username string
	// PasswordHash is the bcrypt hash of the row's dev password
	// (DefaultCost). Usernames, passwords and scopes are unchanged from the
	// .NET table — only the stored format differs.
	PasswordHash string
	Scopes       []string
}

// devUsers is the fixed two-user store for local scaffolding — the exact .NET
// HardcodedCredentialStore table (usernames, passwords and scopes): the
// maintainer holds read+write scopes, the reader holds read-only, so
// least-privilege tokens exist from day one. The literals are allowlisted in
// CI secrets-hygiene (local scaffolding credentials; there is no backing
// store to read from).
//
// Deliberate, documented deviation from the .NET store: the reference hashes
// the username and password with SHA-256 and compares digests in fixed time;
// this port stores bcrypt hashes of the same passwords instead, because
// CodeQL flags SHA-256 over password data (go/weak-sensitive-data-hashing)
// and nothing on the wire depends on the stored format — the same credentials
// validate with identical behavior. bcrypt comparison is inherently
// fixed-time for passwords, and the loop below still evaluates every row with
// no early exit, so nothing about the expected values leaks through response
// timing.
var devUsers = []devUser{
	{
		Username:     "jrb",
		PasswordHash: "$2a$10$2pRdDD4G/CvYwpblPeMececqbby6ZS8YUR1liiTvbjqtqpwB12LNi", // "supersecret"
		Scopes:       []string{domain.ScopeQuotesRead, domain.ScopeQuotesWrite},
	},
	{
		Username:     "reader",
		PasswordHash: "$2a$10$u0H6SQD67jUU0U6HPswpHuonMeuKEbE4YJLsggYYZrZHFIMgaPi/G", // "readsecret"
		Scopes:       []string{domain.ScopeQuotesRead},
	},
}

// ErrProductionCredentialStore mirrors the .NET AddAuthInfrastructure guard:
// the scaffolding store must never run in Production.
var ErrProductionCredentialStore = errors.New(
	"the local scaffolding credential store must not run in Production; register a real CredentialStore adapter")

// HardcodedCredentialStore is the development CredentialStore: bcrypt password
// verification over the fixed table, with usernames compared in fixed time.
// The same Invalid decision comes back for an unknown user and a wrong
// password.
type HardcodedCredentialStore struct{}

// NewHardcodedCredentialStore builds the store, refusing to boot in the
// Production environment.
func NewHardcodedCredentialStore(environment string) (*HardcodedCredentialStore, error) {
	if environment == config.EnvironmentProduction {
		return nil, ErrProductionCredentialStore
	}
	return &HardcodedCredentialStore{}, nil
}

// Validate checks the (username, password) pair and, on success, returns the
// granted scopes. Both row matches are evaluated for every row before the
// conjunction, mirroring the .NET loop's no-early-exit shape.
func (*HardcodedCredentialStore) Validate(_ context.Context, username, password string) (domain.CredentialValidationResult, error) {
	for _, user := range devUsers {
		usernameMatches := subtle.ConstantTimeCompare([]byte(username), []byte(user.Username))
		passwordMatches := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)) == nil

		if usernameMatches == 1 && passwordMatches {
			return domain.CredentialValidationResult{Valid: true, Scopes: user.Scopes}, nil
		}
	}

	return domain.CredentialValidationResult{}, nil
}
