package domain

import "unicode/utf8"

// Account is an internal identity and password record, never an API response.
type Account struct {
	ID           string
	Username     string
	PasswordHash string
}

// Account input errors.
const (
	ErrInvalidUsername ValidationError = "invalid username"
	ErrInvalidPassword ValidationError = "invalid password"
)

// Account and authentication state errors.
const (
	ErrUsernameExists     StateError = "username already exists"
	ErrAccountNotFound    StateError = "account not found"
	ErrBootstrapComplete  StateError = "bootstrap already complete"
	ErrInvalidCredentials StateError = "invalid credentials" // #nosec G101 -- Public error message, not a credential.
	ErrUnauthenticated    StateError = "authentication required"
)

// ValidateUsername accepts lowercase ASCII identifiers without normalization.
func ValidateUsername(username string) error {
	if len(username) < 3 || len(username) > 32 || username[0] < 'a' || username[0] > 'z' {
		return ErrInvalidUsername
	}
	for i := 1; i < len(username); i++ {
		c := username[i]
		if (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '_' && c != '-' {
			return ErrInvalidUsername
		}
	}
	return nil
}

// ValidatePassword enforces UTF-8 and byte bounds without changing the secret.
func ValidatePassword(password []byte) error {
	if len(password) < 12 || len(password) > 128 || !utf8.Valid(password) {
		return ErrInvalidPassword
	}
	return nil
}
