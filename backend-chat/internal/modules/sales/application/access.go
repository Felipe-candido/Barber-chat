package application

import "github.com/google/uuid"

// Produced by the access adapter, never decoded from a request or JWT metadata.
// RecordedByUserID and attribution to a professional are separate concepts.
type SalesAuthority struct {
	Scope          SalesScope
	Owner          bool
	ProfessionalID uuid.UUID
}

func (a SalesAuthority) validate(scope SalesScope) error {
	if scope.ActorID == uuid.Nil || scope.ShopID == uuid.Nil || a.Scope != scope {
		return ErrSalesAccessDenied
	}
	if (a.Owner && a.ProfessionalID != uuid.Nil) || (!a.Owner && a.ProfessionalID == uuid.Nil) {
		return ErrInconsistentSalesData
	}
	return nil
}

func (a SalesAuthority) ProfessionalFor(scope SalesScope, requested uuid.UUID) (uuid.UUID, error) {
	if err := a.validate(scope); err != nil {
		return uuid.Nil, err
	}
	if a.Owner {
		return requested, nil
	}
	if requested != uuid.Nil && requested != a.ProfessionalID {
		return uuid.Nil, ErrSalesAccessDenied
	}
	return a.ProfessionalID, nil
}

func (a SalesAuthority) RequireOwner(scope SalesScope) error {
	if err := a.validate(scope); err != nil {
		return err
	}
	if !a.Owner {
		return ErrSalesAccessDenied
	}
	return nil
}
