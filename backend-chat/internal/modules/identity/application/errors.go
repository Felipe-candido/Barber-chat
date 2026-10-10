package application

import "errors"

var (
	ErrInvalidAccount             = errors.New("invalid staff account input")
	ErrAccountExists              = errors.New("professional already has an account or pending request")
	ErrAccountNotFound            = errors.New("staff account not found")
	ErrAccountNotConfigured       = errors.New("staff account provider is not configured")
	ErrAccountProviderUnavailable = errors.New("staff account provider unavailable")
	ErrAccountProviderRejected    = errors.New("staff account provider rejected the request")
	ErrAccountNeedsReview         = errors.New("account provisioning needs review before retrying")
	ErrAccountMustBeInactive      = errors.New("disable the current barber membership before replacing it")

	ErrInvalidAccessToken   = errors.New("invalid access token")
	ErrInvalidStaffIdentity = errors.New("authenticated user ID is required")
	ErrShopAccessDenied     = errors.New("shop access denied")
	ErrUserNotProvisioned   = errors.New("authenticated user has no local profile")
	ErrUserInactive         = errors.New("user is inactive")

	// ErrIdentityMismatch indicates a repository or resolver contract violation, not an
	// invalid credential. It must be treated as an internal failure.
	ErrIdentityMismatch = errors.New("identity lookup returned inconsistent identifiers")
)
