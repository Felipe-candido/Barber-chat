package application

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

type Authenticate struct {
	verifier   AccessTokenVerifier
	repository IdentityRepository
}

func NewAuthenticate(
	verifier AccessTokenVerifier,
	repository IdentityRepository,
) *Authenticate {
	return &Authenticate{
		verifier:   verifier,
		repository: repository,
	}
}

func (uc *Authenticate) Execute(
	ctx context.Context,
	input ResolveStaffIdentityInput,
) (StaffIdentityOutput, error) {
	if strings.TrimSpace(input.AccessToken) == "" {
		return StaffIdentityOutput{}, ErrInvalidAccessToken
	}

	userID, err := uc.verifier.VerifyAccessToken(ctx, input.AccessToken)
	if err != nil {
		if errors.Is(err, ErrInvalidAccessToken) {
			return StaffIdentityOutput{}, ErrInvalidAccessToken
		}
		return StaffIdentityOutput{}, fmt.Errorf("verify access token: %w", err)
	}

	// A verifier must never authenticate an empty subject.
	if userID == uuid.Nil {
		return StaffIdentityOutput{}, ErrInvalidAccessToken
	}

	user, found, err := uc.repository.FindUserByID(ctx, userID)
	if err != nil {
		return StaffIdentityOutput{}, fmt.Errorf("find local user: %w", err)
	}

	if !found {
		return StaffIdentityOutput{}, ErrUserNotProvisioned
	}

	if user.ID != userID {
		return StaffIdentityOutput{}, ErrIdentityMismatch
	}

	if !user.Active {
		return StaffIdentityOutput{}, ErrUserInactive
	}

	return staffIdentityOutput(user), nil
}
