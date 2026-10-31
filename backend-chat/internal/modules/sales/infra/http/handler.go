package http

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/Felipe-candido/Barber-chat/internal/httpapi/requestctx"
	"github.com/Felipe-candido/Barber-chat/internal/httpapi/response"
	"github.com/Felipe-candido/Barber-chat/internal/modules/sales/application"
	"github.com/Felipe-candido/Barber-chat/internal/modules/sales/domain"
	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
)

type registrar interface {
	Execute(context.Context, application.RegisterSaleInput) (application.SaleOutput, error)
}
type canceller interface {
	Execute(context.Context, application.CancelSaleInput) (application.SaleOutput, error)
}
type queries interface {
	Get(context.Context, application.GetSaleInput) (application.SaleOutput, error)
	List(context.Context, application.ListSalesInput) (application.SalePage, error)
}
type Handler struct {
	register registrar
	cancel   canceller
	queries  queries
	timeout  time.Duration
	logger   *slog.Logger
}

func NewHandler(register registrar, cancel canceller, read queries, timeout time.Duration, logger *slog.Logger) *Handler {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Handler{register: register, cancel: cancel, queries: read, timeout: timeout, logger: logger}
}

type saleItemRequest struct {
	ProductID              uuid.UUID `json:"product_id"`
	Quantity               *int64    `json:"quantity"`
	ExpectedProductVersion *int64    `json:"expected_product_version"`
	ExpectedUnitPriceCents *int64    `json:"expected_unit_price_cents"`
}
type saleRequest struct {
	ProfessionalID uuid.UUID         `json:"professional_id"`
	AppointmentID  uuid.UUID         `json:"appointment_id"`
	Items          []saleItemRequest `json:"items"`
}
type itemResponse struct {
	ID             uuid.UUID `json:"id"`
	ProductID      uuid.UUID `json:"product_id"`
	ProductName    string    `json:"product_name"`
	Quantity       int64     `json:"quantity"`
	UnitPriceCents int64     `json:"unit_price_cents"`
	LineTotalCents int64     `json:"line_total_cents"`
	Currency       string    `json:"currency"`
}
type saleResponse struct {
	ID               uuid.UUID             `json:"id"`
	ShopID           uuid.UUID             `json:"shop_id"`
	RecordedByUserID uuid.UUID             `json:"recorded_by_user_id"`
	ProfessionalID   *uuid.UUID            `json:"professional_id"`
	AppointmentID    *uuid.UUID            `json:"appointment_id"`
	ProfessionalName string                `json:"professional_name"`
	Items            []itemResponse        `json:"items"`
	Status           domain.SaleStatus     `json:"status"`
	Currency         string                `json:"currency"`
	TotalCents       int64                 `json:"total_cents"`
	Version          int64                 `json:"version"`
	SoldAt           time.Time             `json:"sold_at"`
	Replayed         bool                  `json:"replayed"`
	Cancellation     *cancellationResponse `json:"cancellation"`
}

type cancellationResponse struct {
	ActorID     uuid.UUID `json:"actor_id"`
	Reason      string    `json:"reason"`
	Restock     bool      `json:"restock"`
	CancelledAt time.Time `json:"cancelled_at"`
}

func receipt(out application.SaleOutput) saleResponse {
	items := make([]itemResponse, 0, len(out.Items))
	for _, item := range out.Items {
		items = append(items, itemResponse{item.ID, item.ProductID, item.ProductName, item.Quantity, item.UnitPriceCents, item.LineTotalCents, item.Currency})
	}
	r := saleResponse{ID: out.ID, ShopID: out.ShopID, RecordedByUserID: out.RecordedByUserID, ProfessionalID: optionalID(out.ProfessionalID), AppointmentID: optionalID(out.AppointmentID), ProfessionalName: out.ProfessionalName, Items: items, Status: out.Status, Currency: out.Currency, TotalCents: out.TotalCents, Version: out.Version, SoldAt: out.SoldAt, Replayed: out.Replayed}
	if c := out.Cancellation; c != nil {
		r.Cancellation = &cancellationResponse{c.ActorID, c.Reason, c.Restock, c.CancelledAt}
	}
	return r
}

func optionalID(id uuid.UUID) *uuid.UUID {
	if id == uuid.Nil {
		return nil
	}
	return &id
}

func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	actor, authenticated := requestctx.StaffIdentityFromContext(r.Context())
	scope, authorized := requestctx.ShopScopeFromContext(r.Context())
	if !authenticated || !authorized || actor.UserID == uuid.Nil || scope.ShopID == uuid.Nil || scope.UserID != actor.UserID {
		response.Error(w, 403, "access_denied", "staff access required")
		return
	}
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		response.Error(w, 415, "unsupported_media_type", "application/json required")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var body *saleRequest
	err = decoder.Decode(&body)
	if err == nil {
		if trailing := decoder.Decode(new(any)); trailing != io.EOF {
			if trailing != nil {
				err = trailing
			} else {
				err = errors.New("extra JSON data")
			}
		}
	}
	if err != nil || body == nil {
		var max *http.MaxBytesError
		if errors.As(err, &max) {
			response.Error(w, 413, "body_too_large", "body exceeds 16 KiB")
		} else {
			response.Error(w, 400, "invalid_body", "one JSON object with known fields required")
		}
		return
	}
	input := application.RegisterSaleInput{Scope: application.SalesScope{ActorID: actor.UserID, ShopID: scope.ShopID}, ProfessionalID: body.ProfessionalID, AppointmentID: body.AppointmentID, IdempotencyKey: r.Header.Get("Idempotency-Key"), Items: make([]application.SaleItemInput, 0, len(body.Items))}
	for _, item := range body.Items {
		if item.Quantity == nil || item.ExpectedProductVersion == nil || item.ExpectedUnitPriceCents == nil {
			response.Error(w, 400, "invalid_body", "quantity and reviewed price/version required for each item")
			return
		}
		input.Items = append(input.Items, application.SaleItemInput{ProductID: item.ProductID, Quantity: *item.Quantity, ExpectedProductVersion: *item.ExpectedProductVersion, ExpectedUnitPriceCents: *item.ExpectedUnitPriceCents})
	}
	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()
	out, err := h.register.Execute(ctx, input)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	status := 201
	if out.Replayed {
		status = 200
	}
	response.JSON(w, status, receipt(out))
}

func salesRequestScope(w http.ResponseWriter, r *http.Request) (application.SalesScope, bool) {
	a, ok := requestctx.StaffIdentityFromContext(r.Context())
	s, allowed := requestctx.ShopScopeFromContext(r.Context())
	if !ok || !allowed || a.UserID == uuid.Nil || s.ShopID == uuid.Nil || s.UserID != a.UserID {
		response.Error(w, 403, "access_denied", "staff access required")
		return application.SalesScope{}, false
	}
	return application.SalesScope{ActorID: a.UserID, ShopID: s.ShopID}, true
}

func salePathID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, "saleID"))
	if err != nil || id == uuid.Nil {
		response.Error(w, 400, "invalid_sale_id", "valid sale ID required")
		return uuid.Nil, false
	}
	return id, true
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	scope, ok := salesRequestScope(w, r)
	if !ok {
		return
	}
	id, ok := salePathID(w, r)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()
	out, err := h.queries.Get(ctx, application.GetSaleInput{Scope: scope, SaleID: id})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, 200, receipt(out))
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	scope, ok := salesRequestScope(w, r)
	if !ok {
		return
	}
	values, err := urlValues(r)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	input := application.ListSalesInput{Scope: scope, From: values.Get("from"), Through: values.Get("through"), Status: domain.SaleStatus(values.Get("status")), Cursor: values.Get("cursor")}
	if values.Has("limit") {
		input.PageSize, err = strconv.Atoi(values.Get("limit"))
		if err != nil || input.PageSize < 1 {
			h.fail(w, r, application.ErrInvalidSalesQuery)
			return
		}
	}
	if values.Has("professional_id") {
		input.ProfessionalID, err = uuid.Parse(values.Get("professional_id"))
		if err != nil || input.ProfessionalID == uuid.Nil {
			h.fail(w, r, application.ErrInvalidSalesQuery)
			return
		}
	}
	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()
	out, err := h.queries.List(ctx, input)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	items := make([]saleResponse, 0, len(out.Items))
	for _, sale := range out.Items {
		items = append(items, receipt(sale))
	}
	response.JSON(w, 200, struct {
		Items      []saleResponse `json:"items"`
		NextCursor string         `json:"next_cursor"`
		Timezone   string         `json:"timezone"`
	}{items, out.NextCursor, out.Timezone})
}

func urlValues(r *http.Request) (url.Values, error) {
	values, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return nil, application.ErrInvalidSalesQuery
	}
	for key, v := range values {
		if len(v) != 1 {
			return nil, application.ErrInvalidSalesQuery
		}
		switch key {
		case "from", "through", "status", "professional_id", "limit", "cursor":
		default:
			return nil, application.ErrInvalidSalesQuery
		}
	}
	return values, nil
}

func (h *Handler) Cancel(w http.ResponseWriter, r *http.Request) {
	scope, ok := salesRequestScope(w, r)
	if !ok {
		return
	}
	id, ok := salePathID(w, r)
	if !ok {
		return
	}
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		response.Error(w, 415, "unsupported_media_type", "application/json required")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var body *struct {
		ExpectedVersion *int64 `json:"expected_version"`
		Reason          string `json:"reason"`
		Restock         *bool  `json:"restock"`
	}
	err = decoder.Decode(&body)
	if err == nil {
		if trailing := decoder.Decode(new(any)); trailing != io.EOF {
			if trailing != nil {
				err = trailing
			} else {
				err = errors.New("extra JSON data")
			}
		}
	}
	if err != nil || body == nil || body.ExpectedVersion == nil || body.Restock == nil {
		var max *http.MaxBytesError
		if errors.As(err, &max) {
			response.Error(w, 413, "body_too_large", "body exceeds 16 KiB")
		} else {
			response.Error(w, 400, "invalid_body", "expected_version, reason and explicit restock boolean required in one JSON object")
		}
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()
	out, err := h.cancel.Execute(ctx, application.CancelSaleInput{Scope: scope, SaleID: id, ExpectedVersion: *body.ExpectedVersion, Reason: body.Reason, Restock: *body.Restock, IdempotencyKey: r.Header.Get("Idempotency-Key")})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, 200, receipt(out))
}

func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, application.ErrSaleNotFound):
		response.Error(w, 404, "sale_not_found", "sale not found")
	case errors.Is(err, application.ErrInvalidSalesQuery):
		response.Error(w, 400, "invalid_sales_query", "review dates, filters, limit and cursor")
	case errors.Is(err, domain.ErrSaleVersionConflict):
		response.Error(w, 409, "sale_version_changed", "reload the sale before cancelling")
	case errors.Is(err, domain.ErrSaleAlreadyCancelled):
		response.Error(w, 409, "sale_already_cancelled", "sale already cancelled with different instructions")
	case errors.Is(err, application.ErrSaleStockLimitExceeded):
		response.Error(w, 409, "stock_limit_exceeded", "stock return exceeds the supported quantity; cancellation was not saved")
	case errors.Is(err, application.ErrSalesAccessDenied):
		response.Error(w, 403, "access_denied", "sale access denied")
	case errors.Is(err, application.ErrInvalidSaleCommandKey):
		response.Error(w, 400, "invalid_idempotency_key", "64 lowercase hex characters required")
	case errors.Is(err, application.ErrInvalidSalesInput), errors.Is(err, domain.ErrInvalidSale), errors.Is(err, domain.ErrInvalidSaleItem), errors.Is(err, domain.ErrInvalidSaleAmount):
		response.Error(w, 422, "invalid_sale", "review products, quantities and expected prices")
	case errors.Is(err, application.ErrSaleSubscriptionRequired):
		scope, _ := requestctx.ShopScopeFromContext(r.Context())
		status := 403
		if scope.Role == "owner" {
			status = 402
		}
		response.Error(w, status, "shop_subscription_required", "current subscription coverage required for a new sale")
	case errors.Is(err, application.ErrSalePriceChanged):
		response.Error(w, 409, "sale_price_changed", "reload products and review current prices")
	case errors.Is(err, application.ErrSaleProductUnavailable):
		response.Error(w, 409, "sale_product_unavailable", "selected product is unavailable")
	case errors.Is(err, application.ErrSaleStockUnavailable):
		response.Error(w, 409, "insufficient_stock", "not enough product stock; no sale recorded")
	case errors.Is(err, application.ErrSaleAppointmentUnavailable):
		response.Error(w, 409, "sale_appointment_unavailable", "appointment cannot be linked to this sale")
	case errors.Is(err, application.ErrSaleCommandConflict):
		response.Error(w, 409, "idempotency_conflict", "key used with different sale input")
	case errors.Is(err, application.ErrSaleCommandPending):
		response.Error(w, 409, "sale_command_pending", "retry same key/input after checking the result")
	default:
		h.logger.ErrorContext(r.Context(), "sale request could not be completed", "request_id", chimiddleware.GetReqID(r.Context()))
		response.Error(w, 503, "sale_unavailable", "request could not be completed; for writes retry original key and input")
	}
}
