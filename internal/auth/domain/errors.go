// Package domain holds the auth bounded context's core model: the credential
// validation port, the token issuance port with its result model, and the
// canonical auth error codes. It imports nothing from outside the standard
// library.
//
// Failures are *domain.Error values carrying a stable, wire-visible Code — the
// codes are part of the public API contract (they surface as the errorCode
// field of error envelopes), so renaming one is a breaking change. The
// transport layer maps Code to an HTTP status (auth.invalid_credentials →
// 401); rate limiting and other transport-owned codes (for example
// auth.token_missing) live with the transport, not here.
package domain

// Error is a canonical domain failure: a stable code the transport maps to an
// HTTP status, plus a human-readable description.
type Error struct {
	Code        string
	Description string
}

// Error implements the error interface; the message carries both the code and
// the description so logs can grep either.
func (e *Error) Error() string {
	return e.Code + ": " + e.Description
}

// InvalidCredentials returns the canonical error for rejected credentials —
// the same answer for an unknown user and a wrong password, so the pair
// cannot be distinguished from the outside.
func InvalidCredentials() *Error {
	return &Error{
		Code:        "auth.invalid_credentials",
		Description: "Invalid credentials.",
	}
}
