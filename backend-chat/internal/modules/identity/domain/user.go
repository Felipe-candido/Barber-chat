package domain

import (
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
)

type User struct {
	ID          uuid.UUID
	DisplayName string
	Active      bool
}

func NewUser(userID uuid.UUID, displayName string) (User, error) {
	displayName = strings.TrimSpace(displayName)

	if userID == uuid.Nil {
		return User{}, ErrInvalidUserID
	}

	if displayName == "" || utf8.RuneCountInString(displayName) > 100 {
		return User{}, ErrInvalidDisplayName
	}

	return User{
		ID:          userID,
		DisplayName: displayName,
		Active:      true,
	}, nil
}
