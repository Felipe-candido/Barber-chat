package application

import (
	"github.com/Felipe-candido/Barber-chat/internal/modules/customers/domain"
	"github.com/google/uuid"
	"time"
)

type ContactScope struct{ ActorID, ShopID uuid.UUID }
type CreateContactInput struct{ FirstName, LastName, Phone, PhoneCountry string }
type ListContactsInput struct {
	Name, Phone, PhoneCountry, Cursor string
	Limit                             int
}
type ContactCursor struct {
	ShopID    uuid.UUID `json:"shop"`
	CreatedAt time.Time `json:"at"`
	ID        uuid.UUID `json:"id"`
	Filter    string    `json:"filter"`
	Version   int       `json:"v"`
}
type ContactQuery struct {
	Name, Phone string
	After       *ContactCursor
	Limit       int
}
type ContactPage struct {
	Items      []domain.Contact
	NextCursor string
}
