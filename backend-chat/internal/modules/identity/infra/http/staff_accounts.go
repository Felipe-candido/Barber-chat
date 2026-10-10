package http

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"time"

	"github.com/Felipe-candido/Barber-chat/internal/httpapi/requestctx"
	"github.com/Felipe-candido/Barber-chat/internal/httpapi/response"
	"github.com/Felipe-candido/Barber-chat/internal/modules/identity/application"
	"github.com/Felipe-candido/Barber-chat/internal/modules/identity/domain"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type AccountHandler struct {
	accounts *application.StaffAccounts
	timeout  time.Duration
	logger   *slog.Logger
}

func NewAccountHandler(accounts *application.StaffAccounts, timeout time.Duration, logger *slog.Logger) *AccountHandler {
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &AccountHandler{accounts: accounts, timeout: timeout, logger: logger}
}

type accountResponse struct {
	ID             uuid.UUID  `json:"id"`
	ProfessionalID uuid.UUID  `json:"professional_id"`
	Email          string     `json:"email"`
	UserID         *uuid.UUID `json:"user_id"`
	State          string     `json:"state"`
	Active         bool       `json:"active"`
}

func accountJSON(a application.StaffAccount) accountResponse {
	var userID *uuid.UUID
	if a.UserID != uuid.Nil {
		userID = &a.UserID
	}
	return accountResponse{a.ID, a.ProfessionalID, a.Email, userID, a.State, a.Active}
}

func ownerScope(w http.ResponseWriter, r *http.Request) (application.AuthorizedShopScope, bool) {
	scope, ok := requestctx.ShopScopeFromContext(r.Context())
	identity, authenticated := requestctx.StaffIdentityFromContext(r.Context())
	if !ok || !authenticated || identity.UserID == uuid.Nil || scope.UserID != identity.UserID || scope.ShopID == uuid.Nil || scope.Role != domain.RoleOwner {
		response.Error(w, 403, "access_denied", "owner access is required")
		return application.AuthorizedShopScope{}, false
	}
	return scope, true
}

func decodeAccount(w http.ResponseWriter, r *http.Request, target any) bool {
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		response.Error(w, 415, "unsupported_media_type", "application/json is required")
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, 8*1024)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	err = d.Decode(target)
	if err == nil {
		var extra any
		if nextErr := d.Decode(&extra); nextErr != io.EOF {
			err = nextErr
			if err == nil {
				err = errors.New("extra JSON value")
			}
		}
	}
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			response.Error(w, 413, "body_too_large", "body exceeds limit")
		} else {
			response.Error(w, 400, "invalid_body", "one valid JSON object is required")
		}
		return false
	}
	return true
}

func (h *AccountHandler) List(w http.ResponseWriter, r *http.Request) {
	scope, ok := ownerScope(w, r)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()
	items, err := h.accounts.List(ctx, scope.UserID, scope.ShopID)
	if err != nil {
		h.failure(w, r, err)
		return
	}
	result := make([]accountResponse, 0, len(items))
	for _, a := range items {
		result = append(result, accountJSON(a))
	}
	response.JSON(w, 200, result)
}

func (h *AccountHandler) Invite(w http.ResponseWriter, r *http.Request) {
	scope, ok := ownerScope(w, r)
	if !ok {
		return
	}
	var input *struct {
		ProfessionalID uuid.UUID `json:"professional_id"`
		Email          string    `json:"email"`
	}
	if !decodeAccount(w, r, &input) {
		return
	}
	if input == nil {
		response.Error(w, 400, "invalid_body", "JSON object required")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()
	account, err := h.accounts.Invite(ctx, scope.UserID, scope.ShopID, input.ProfessionalID, input.Email)
	if err != nil {
		h.failure(w, r, err)
		return
	}
	response.JSON(w, 201, accountJSON(account))
}

func (h *AccountHandler) Replace(w http.ResponseWriter, r *http.Request) {
	scope, ok := ownerScope(w, r)
	if !ok {
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "accountID"))
	if err != nil || id == uuid.Nil {
		h.failure(w, r, application.ErrInvalidAccount)
		return
	}
	var input *struct {
		Email string `json:"email"`
	}
	if !decodeAccount(w, r, &input) {
		return
	}
	if input == nil {
		response.Error(w, 400, "invalid_body", "JSON object required")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()
	account, err := h.accounts.Replace(ctx, scope.UserID, scope.ShopID, id, input.Email)
	if err != nil {
		h.failure(w, r, err)
		return
	}
	response.JSON(w, 201, accountJSON(account))
}

func (h *AccountHandler) Link(w http.ResponseWriter, r *http.Request) {
	scope, ok := ownerScope(w, r)
	if !ok {
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "accountID"))
	if err != nil || id == uuid.Nil {
		h.failure(w, r, application.ErrInvalidAccount)
		return
	}
	var input *struct {
		UserID uuid.UUID `json:"user_id"`
	}
	if !decodeAccount(w, r, &input) {
		return
	}
	if input == nil {
		response.Error(w, 400, "invalid_body", "JSON object required")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()
	account, err := h.accounts.LinkExisting(ctx, scope.UserID, scope.ShopID, id, input.UserID)
	if err != nil {
		h.failure(w, r, err)
		return
	}
	response.JSON(w, 200, accountJSON(account))
}

func (h *AccountHandler) SetActive(w http.ResponseWriter, r *http.Request) {
	scope, ok := ownerScope(w, r)
	if !ok {
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "accountID"))
	if err != nil || id == uuid.Nil {
		h.failure(w, r, application.ErrInvalidAccount)
		return
	}
	var input *struct {
		Active *bool `json:"active"`
	}
	if !decodeAccount(w, r, &input) {
		return
	}
	if input == nil || input.Active == nil {
		h.failure(w, r, application.ErrInvalidAccount)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()
	if err = h.accounts.SetActive(ctx, scope.UserID, scope.ShopID, id, *input.Active); err != nil {
		h.failure(w, r, err)
		return
	}
	response.JSON(w, 200, struct {
		Active bool `json:"active"`
	}{*input.Active})
}

func (h *AccountHandler) failure(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, application.ErrInvalidAccount):
		response.Error(w, 422, "invalid_staff_account", "check email and professional")
	case errors.Is(err, application.ErrAccountExists):
		response.Error(w, 409, "staff_account_exists", "account or request already exists; refresh before retrying")
	case errors.Is(err, application.ErrAccountMustBeInactive):
		response.Error(w, 409, "staff_account_must_be_inactive", "disable the current access before replacing it")
	case errors.Is(err, application.ErrAccountNotFound):
		response.Error(w, 404, "staff_account_not_found", "account or professional not found")
	case errors.Is(err, application.ErrAccountNeedsReview):
		response.Error(w, 409, "staff_account_needs_review", "review the existing request and Supabase account before retrying")
	case errors.Is(err, application.ErrShopAccessDenied), errors.Is(err, application.ErrUserInactive):
		response.Error(w, 403, "access_denied", "owner access and active records are required")
	case errors.Is(err, application.ErrAccountNotConfigured):
		response.Error(w, 503, "staff_accounts_not_configured", "account creation is not configured")
	case errors.Is(err, application.ErrAccountProviderRejected):
		response.Error(w, 422, "staff_account_provider_rejected", "the Auth account could not be verified")
	case errors.Is(err, application.ErrAccountProviderUnavailable), errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		response.Error(w, 503, "temporarily_unavailable", "account operation unavailable")
	default:
		h.logger.ErrorContext(r.Context(), "staff account operation failed")
		response.Error(w, 500, "internal_error", "account operation could not be completed")
	}
}
