package application

import "errors"

var (
	ErrInvalidAccessToken   = errors.New("invalid access token")
	ErrInvalidStaffIdentity = errors.New("authenticated user ID is required")
	ErrShopAccessDenied     = errors.New("shop access denied")
	ErrUserNotProvisioned   = errors.New("authenticated user has no local profile")
	ErrUserInactive         = errors.New("user is inactive")

	// ErrIdentityMismatch indicates a repository or resolver contract violation, not an
	// invalid credential. It must be treated as an internal failure.
	ErrIdentityMismatch = errors.New("identity lookup returned inconsistent identifiers")
)
