package domain

import "context"

// CredentialValidationResult is the outcome of a credential check. A valid
// decision carries the scopes granted to the principal, so the caller never
// has to infer authorization from the username. The zero value is the invalid
// decision.
type CredentialValidationResult struct {
	Valid  bool
	Scopes []string
}

// CredentialStore is the port for looking up credentials — a hardcoded table
// in development, a remote identity in production. Implementations must not
// block callers for long (hashing stays off the hot path) and must return the
// same Invalid decision for an unknown user and a wrong password.
type CredentialStore interface {
	// Validate checks the (username, password) pair and, on success, returns
	// the granted scopes.
	Validate(ctx context.Context, username, password string) (CredentialValidationResult, error)
}
