package application

import "errors"

var (
	ErrInvalidRegistration    = errors.New("invalid shop registration")
	ErrOwnerInactive          = errors.New("owner account inactive")
	ErrOwnershipDenied        = errors.New("shop ownership denied")
	ErrSlugTaken              = errors.New("shop slug already in use")
	ErrCreationConflict       = errors.New("shop creation key reused with different input")
	ErrCreationMismatch       = errors.New("shop creation identity mismatch")
	ErrSettingsAccessDenied   = errors.New("shop settings access denied")
	ErrSettingsNotFound       = errors.New("shop settings not found")
	ErrSettingsConflict       = errors.New("shop settings changed; reload before updating")
	ErrInvalidSettingsVersion = errors.New("invalid shop settings version")
	ErrSettingsMismatch       = errors.New("shop settings identity mismatch")
)
