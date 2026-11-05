package application

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"github.com/Felipe-candido/Barber-chat/internal/modules/customers/domain"
	"github.com/google/uuid"
	"io"
	"strings"
	"unicode"
	"unicode/utf8"
)

type ManageContacts struct {
	tx     ContactTransactor
	phones ContactPhoneNormalizer
}

func NewManageContacts(tx ContactTransactor, phones ContactPhoneNormalizer) *ManageContacts {
	return &ManageContacts{tx, phones}
}

func (u *ManageContacts) authorized(ctx context.Context, s ContactScope, fn func(context.Context, ContactRepository) error) error {
	if s.ActorID == uuid.Nil || s.ShopID == uuid.Nil {
		return ErrContactAccessDenied
	}
	return u.tx.WithinContactTx(ctx, func(ctx context.Context, tx ContactTx) error {
		if err := tx.Access.RequireContactsAccess(ctx, s); err != nil {
			return err
		}
		return fn(ctx, tx.Contacts)
	})
}
func (u *ManageContacts) Create(ctx context.Context, s ContactScope, in CreateContactInput) (domain.Contact, error) {
	var result domain.Contact
	err := u.authorized(ctx, s, func(ctx context.Context, r ContactRepository) error {
		in.FirstName = strings.TrimSpace(in.FirstName)
		in.LastName = strings.TrimSpace(in.LastName)
		if !domain.ValidName(in.FirstName, false) || !domain.ValidName(in.LastName, true) {
			return domain.ErrInvalidContact
		}
		phone, err := u.phones.NormalizeContactPhone(in.Phone, in.PhoneCountry)
		if err != nil {
			return domain.ErrInvalidContact
		}
		id, err := uuid.NewRandom()
		if err != nil {
			return err
		}
		result, err = r.Create(ctx, domain.Contact{ID: id, ShopID: s.ShopID, FirstName: in.FirstName, LastName: in.LastName, Phone: phone})
		return err
	})
	return result, err
}
func (u *ManageContacts) Get(ctx context.Context, s ContactScope, id uuid.UUID) (domain.Contact, error) {
	var result domain.Contact
	err := u.authorized(ctx, s, func(ctx context.Context, r ContactRepository) error {
		if id == uuid.Nil {
			return ErrContactNotFound
		}
		c, found, err := r.Find(ctx, s.ShopID, id)
		if err != nil {
			return err
		}
		if !found {
			return ErrContactNotFound
		}
		result = c
		return nil
	})
	return result, err
}
func (u *ManageContacts) List(ctx context.Context, s ContactScope, in ListContactsInput) (ContactPage, error) {
	result := ContactPage{Items: []domain.Contact{}}
	err := u.authorized(ctx, s, func(ctx context.Context, r ContactRepository) error {
		limit := in.Limit
		if limit == 0 {
			limit = 25
		}
		if limit < 1 || limit > 100 {
			return ErrInvalidContactQuery
		}
		name := strings.TrimSpace(in.Name)
		if !utf8.ValidString(name) || utf8.RuneCountInString(name) > 201 || strings.ContainsFunc(name, unicode.IsControl) {
			return ErrInvalidContactQuery
		}
		phone := ""
		if strings.TrimSpace(in.Phone) != "" {
			var err error
			phone, err = u.phones.NormalizeContactPhone(in.Phone, in.PhoneCountry)
			if err != nil {
				return ErrInvalidContactQuery
			}
		} else if in.PhoneCountry != "" {
			return ErrInvalidContactQuery
		}
		filterBytes, _ := json.Marshal([]string{name, phone})
		digest := sha256.Sum256(filterBytes)
		filter := hex.EncodeToString(digest[:])
		q := ContactQuery{Name: name, Phone: phone, Limit: limit + 1}
		if in.Cursor != "" {
			if len(in.Cursor) > 512 {
				return ErrInvalidContactQuery
			}
			raw, err := base64.RawURLEncoding.Strict().DecodeString(in.Cursor)
			if err != nil {
				return ErrInvalidContactQuery
			}
			d := json.NewDecoder(bytes.NewReader(raw))
			d.DisallowUnknownFields()
			var c ContactCursor
			if d.Decode(&c) != nil || d.Decode(new(any)) != io.EOF || c.Version != 1 || c.ShopID != s.ShopID || c.ID == uuid.Nil || c.CreatedAt.IsZero() || c.CreatedAt.Year() < 1 || c.CreatedAt.Year() > 9999 || c.Filter != filter {
				return ErrInvalidContactQuery
			}
			q.After = &c
		}
		items, err := r.List(ctx, s.ShopID, q)
		if err != nil {
			return err
		}
		if len(items) > limit {
			items = items[:limit]
			last := items[len(items)-1]
			raw, _ := json.Marshal(ContactCursor{ShopID: s.ShopID, ID: last.ID, CreatedAt: last.CreatedAt, Filter: filter, Version: 1})
			result.NextCursor = base64.RawURLEncoding.EncodeToString(raw)
		}
		if items != nil {
			result.Items = items
		}
		return nil
	})
	return result, err
}
