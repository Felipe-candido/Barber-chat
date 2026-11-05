package http

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/Felipe-candido/Barber-chat/internal/httpapi/requestctx"
	"github.com/Felipe-candido/Barber-chat/internal/httpapi/response"
	app "github.com/Felipe-candido/Barber-chat/internal/modules/customers/application"
	"github.com/Felipe-candido/Barber-chat/internal/modules/customers/domain"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

type ContactHandler struct {
	usecase *app.ManageContacts
	timeout time.Duration
	logger  *slog.Logger
}

func NewContactHandler(uc *app.ManageContacts, timeout time.Duration, logger *slog.Logger) *ContactHandler {
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &ContactHandler{uc, timeout, logger}
}

type contactJSON struct {
	ID        uuid.UUID `json:"id"`
	ShopID    uuid.UUID `json:"shop_id"`
	FirstName string    `json:"first_name"`
	LastName  string    `json:"last_name"`
	Name      string    `json:"name"`
	Phone     string    `json:"phone"`
	CreatedAt time.Time `json:"created_at"`
}

func contactBody(c domain.Contact) contactJSON {
	return contactJSON{c.ID, c.ShopID, c.FirstName, c.LastName, c.Name(), c.Phone, c.CreatedAt}
}
func contactScope(r *http.Request) (app.ContactScope, bool) {
	actor, a := requestctx.StaffIdentityFromContext(r.Context())
	shop, b := requestctx.ShopScopeFromContext(r.Context())
	return app.ContactScope{ActorID: actor.UserID, ShopID: shop.ShopID}, a && b && actor.UserID != uuid.Nil && shop.ShopID != uuid.Nil && actor.UserID == shop.UserID && shop.Role.Valid()
}
func (h *ContactHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	s, ok := contactScope(r)
	if !ok {
		h.fail(w, app.ErrContactAccessDenied)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()
	if r.Method == http.MethodPost {
		media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || media != "application/json" {
			response.Error(w, 415, "unsupported_media_type", "Content-Type must be application/json")
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 2048)
		d := json.NewDecoder(r.Body)
		d.DisallowUnknownFields()
		var body *struct {
			FirstName    string `json:"first_name"`
			LastName     string `json:"last_name"`
			Phone        string `json:"phone"`
			PhoneCountry string `json:"phone_country"`
		}
		if err = d.Decode(&body); err != nil || body == nil || d.Decode(new(any)) != io.EOF {
			var large *http.MaxBytesError
			if errors.As(err, &large) {
				response.Error(w, 413, "body_too_large", "contact body exceeds limit")
			} else {
				response.Error(w, 400, "invalid_body", "one contact JSON object with known fields is required")
			}
			return
		}
		c, err := h.usecase.Create(ctx, s, app.CreateContactInput{FirstName: body.FirstName, LastName: body.LastName, Phone: body.Phone, PhoneCountry: body.PhoneCountry})
		if err != nil {
			h.fail(w, err)
			return
		}
		response.JSON(w, 201, contactBody(c))
		return
	}
	if raw := chi.URLParam(r, "contactID"); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			h.fail(w, app.ErrContactNotFound)
			return
		}
		c, err := h.usecase.Get(ctx, s, id)
		if err != nil {
			h.fail(w, err)
			return
		}
		response.JSON(w, 200, contactBody(c))
		return
	}
	q, err := parseContactQuery(r)
	if err != nil {
		h.fail(w, err)
		return
	}
	page, err := h.usecase.List(ctx, s, q)
	if err != nil {
		h.fail(w, err)
		return
	}
	items := make([]contactJSON, 0, len(page.Items))
	for _, c := range page.Items {
		items = append(items, contactBody(c))
	}
	var next *string
	if page.NextCursor != "" {
		next = &page.NextCursor
	}
	response.JSON(w, 200, struct {
		Items []contactJSON `json:"items"`
		Next  *string       `json:"next_cursor"`
	}{items, next})
}
func parseContactQuery(r *http.Request) (app.ListContactsInput, error) {
	values, err := urlQuery(r)
	if err != nil {
		return app.ListContactsInput{}, err
	}
	in := app.ListContactsInput{Name: values.Get("name"), Phone: values.Get("phone"), PhoneCountry: values.Get("phone_country"), Cursor: values.Get("cursor")}
	if raw, ok := values["limit"]; ok {
		if raw[0] == "" {
			return in, app.ErrInvalidContactQuery
		}
		in.Limit, err = strconv.Atoi(raw[0])
		if err != nil || in.Limit < 1 || in.Limit > 100 {
			return in, app.ErrInvalidContactQuery
		}
	}
	return in, nil
}
func urlQuery(r *http.Request) (url.Values, error) {
	if len(r.URL.RawQuery) > 2048 {
		return nil, app.ErrInvalidContactQuery
	}
	values, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return nil, app.ErrInvalidContactQuery
	}
	for key, items := range values {
		switch key {
		case "name", "phone", "phone_country", "cursor", "limit":
		default:
			return nil, app.ErrInvalidContactQuery
		}
		if len(items) != 1 {
			return nil, app.ErrInvalidContactQuery
		}
	}
	return values, nil
}
func (h *ContactHandler) fail(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrInvalidContact):
		response.Error(w, 422, "invalid_contact", "check contact name and phone")
	case errors.Is(err, app.ErrInvalidContactQuery):
		response.Error(w, 400, "invalid_contact_query", "check search, cursor and page limit")
	case errors.Is(err, app.ErrContactAccessDenied):
		response.Error(w, 403, "contact_access_denied", "active shop membership is required")
	case errors.Is(err, app.ErrContactExists):
		response.Error(w, 409, "contact_exists", "a contact with this phone already exists in this shop")
	case errors.Is(err, app.ErrContactNotFound):
		response.Error(w, 404, "contact_not_found", "contact not found")
	default:
		h.logger.Warn("contact request could not be completed")
		response.Error(w, 503, "contact_unavailable", "contact request could not be completed")
	}
}
