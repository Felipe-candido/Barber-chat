package application

import (
	"context"
	"errors"
	"github.com/Felipe-candido/Barber-chat/internal/modules/customers/domain"
	"github.com/google/uuid"
)

var (
	ErrContactAccessDenied = errors.New("contact access denied")
	ErrContactExists       = errors.New("contact phone already exists")
	ErrContactNotFound     = errors.New("contact not found")
	ErrInvalidContactQuery = errors.New("invalid contact query")
)

type ContactPhoneNormalizer interface {
	NormalizeContactPhone(string, string) (string, error)
}
type ContactAccess interface {
	RequireContactsAccess(context.Context, ContactScope) error
}
type ContactRepository interface {
	Create(context.Context, domain.Contact) (domain.Contact, error)
	Find(context.Context, uuid.UUID, uuid.UUID) (domain.Contact, bool, error)
	List(context.Context, uuid.UUID, ContactQuery) ([]domain.Contact, error)
}
type ContactTx struct {
	Access   ContactAccess
	Contacts ContactRepository
}
type ContactTransactor interface {
	WithinContactTx(context.Context, func(context.Context, ContactTx) error) error
}
