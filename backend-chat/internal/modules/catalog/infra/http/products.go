package http

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Felipe-candido/Barber-chat/internal/httpapi/requestctx"
	"github.com/Felipe-candido/Barber-chat/internal/httpapi/response"
	"github.com/Felipe-candido/Barber-chat/internal/modules/catalog/application"
	"github.com/Felipe-candido/Barber-chat/internal/modules/catalog/domain"
	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
)

type ProductHandler struct {
	commands *application.ProductCommands
	queries  *application.ProductQueries
	timeout  time.Duration
	logger   *slog.Logger
}

func NewProductHandler(commands *application.ProductCommands, queries *application.ProductQueries, timeout time.Duration, logger *slog.Logger) *ProductHandler {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &ProductHandler{commands, queries, timeout, logger}
}

type productResponse struct {
	ID              uuid.UUID `json:"id"`
	ShopID          uuid.UUID `json:"shop_id"`
	Name            string    `json:"name"`
	Description     string    `json:"description"`
	PriceCents      int64     `json:"price_cents"`
	Currency        string    `json:"currency"`
	Active          bool      `json:"active"`
	Version         int64     `json:"version"`
	QuantityOnHand  int64     `json:"quantity_on_hand"`
	MinimumQuantity int64     `json:"minimum_quantity"`
	StockVersion    int64     `json:"stock_version"`
	LowStock        bool      `json:"low_stock"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

func productResponseOf(p application.ProductOutput) productResponse {
	return productResponse{p.ID, p.ShopID, p.Name, p.Description, p.PriceCents, p.Currency, p.Active, p.Version, p.QuantityOnHand, p.MinimumQuantity, p.StockVersion, p.QuantityOnHand <= p.MinimumQuantity, p.CreatedAt, p.UpdatedAt}
}

func productRequestScope(w http.ResponseWriter, r *http.Request) (application.ProductScope, bool) {
	actor, authenticated := requestctx.StaffIdentityFromContext(r.Context())
	scope, authorized := requestctx.ShopScopeFromContext(r.Context())
	if !authenticated || !authorized || actor.UserID == uuid.Nil || actor.UserID != scope.UserID || scope.ShopID == uuid.Nil {
		response.Error(w, 403, "access_denied", "staff access is required")
		return application.ProductScope{}, false
	}
	return application.ProductScope{ActorID: actor.UserID, ShopID: scope.ShopID}, true
}
func decodeProductBody(w http.ResponseWriter, r *http.Request, target any) bool {
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		response.Error(w, 415, "unsupported_media_type", "application/json is required")
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, 8<<10)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	err = decoder.Decode(target)
	if err == nil {
		if decoder.Decode(new(any)) != io.EOF {
			err = errors.New("additional JSON data")
		}
	}
	if err != nil {
		var limit *http.MaxBytesError
		if errors.As(err, &limit) {
			response.Error(w, 413, "body_too_large", "body exceeds 8 KiB")
		} else {
			response.Error(w, 400, "invalid_body", "one JSON object with known fields is required")
		}
		return false
	}
	return true
}
func productPathID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, "productID"))
	if err != nil || id == uuid.Nil {
		response.Error(w, 400, "invalid_product_id", "valid product ID is required")
		return uuid.Nil, false
	}
	return id, true
}

func (h *ProductHandler) Create(w http.ResponseWriter, r *http.Request) {
	scope, ok := productRequestScope(w, r)
	if !ok {
		return
	}
	var body *struct {
		Name            string `json:"name"`
		Description     string `json:"description"`
		PriceCents      *int64 `json:"price_cents"`
		InitialQuantity *int64 `json:"initial_quantity"`
		MinimumQuantity *int64 `json:"minimum_quantity"`
	}
	if !decodeProductBody(w, r, &body) {
		return
	}
	if body == nil || body.PriceCents == nil || body.InitialQuantity == nil || body.MinimumQuantity == nil {
		response.Error(w, 400, "invalid_body", "price_cents, initial_quantity and minimum_quantity are required")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()
	out, err := h.commands.Create(ctx, application.CreateProductInput{Scope: scope, Name: body.Name, Description: body.Description, PriceCents: *body.PriceCents, InitialQuantity: *body.InitialQuantity, MinimumQuantity: *body.MinimumQuantity, IdempotencyKey: r.Header.Get("Idempotency-Key")})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.mutation(w, out)
}
func (h *ProductHandler) RecordStock(w http.ResponseWriter, r *http.Request) {
	scope, ok := productRequestScope(w, r)
	if !ok {
		return
	}
	id, ok := productPathID(w, r)
	if !ok {
		return
	}
	var body *struct {
		Operation     application.ManualStockOperation `json:"operation"`
		QuantityDelta *int64                           `json:"quantity_delta"`
		Reason        string                           `json:"reason"`
	}
	if !decodeProductBody(w, r, &body) {
		return
	}
	if body == nil || body.QuantityDelta == nil {
		response.Error(w, 400, "invalid_body", "quantity_delta is required")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()
	out, err := h.commands.RecordStock(ctx, application.RecordStockInput{Scope: scope, ProductID: id, Operation: body.Operation, QuantityDelta: *body.QuantityDelta, Reason: body.Reason, IdempotencyKey: r.Header.Get("Idempotency-Key")})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.mutation(w, out)
}
func (h *ProductHandler) mutation(w http.ResponseWriter, out application.ProductMutationOutput) {
	var movementID *uuid.UUID
	if out.MovementID != uuid.Nil {
		id := out.MovementID
		movementID = &id
	}
	status := 201
	if out.Replayed {
		status = 200
	}
	response.JSON(w, status, struct {
		Product    productResponse `json:"product"`
		MovementID *uuid.UUID      `json:"movement_id"`
		Replayed   bool            `json:"replayed"`
	}{productResponseOf(out.Product), movementID, out.Replayed})
}
func (h *ProductHandler) Update(w http.ResponseWriter, r *http.Request) {
	scope, ok := productRequestScope(w, r)
	if !ok {
		return
	}
	id, ok := productPathID(w, r)
	if !ok {
		return
	}
	var body *struct {
		Name            *string `json:"name"`
		Description     *string `json:"description"`
		PriceCents      *int64  `json:"price_cents"`
		Active          *bool   `json:"active"`
		ExpectedVersion *int64  `json:"expected_version"`
	}
	if !decodeProductBody(w, r, &body) {
		return
	}
	if body == nil || body.Name == nil || body.Description == nil || body.PriceCents == nil || body.Active == nil || body.ExpectedVersion == nil {
		response.Error(w, 400, "invalid_body", "all editable fields and expected_version are required")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()
	out, err := h.commands.Update(ctx, application.UpdateProductInput{Scope: scope, ProductID: id, Details: domain.ProductDetails{Name: *body.Name, Description: *body.Description, PriceCents: *body.PriceCents, Active: *body.Active}, ExpectedVersion: *body.ExpectedVersion})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, 200, productResponseOf(out))
}
func (h *ProductHandler) SetMinimum(w http.ResponseWriter, r *http.Request) {
	scope, ok := productRequestScope(w, r)
	if !ok {
		return
	}
	id, ok := productPathID(w, r)
	if !ok {
		return
	}
	var body *struct {
		MinimumQuantity *int64 `json:"minimum_quantity"`
		ExpectedVersion *int64 `json:"expected_stock_version"`
	}
	if !decodeProductBody(w, r, &body) {
		return
	}
	if body == nil || body.MinimumQuantity == nil || body.ExpectedVersion == nil {
		response.Error(w, 400, "invalid_body", "minimum_quantity and expected_stock_version are required")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()
	out, err := h.commands.SetMinimum(ctx, application.UpdateStockMinimumInput{Scope: scope, ProductID: id, MinimumQuantity: *body.MinimumQuantity, ExpectedStockVersion: *body.ExpectedVersion})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, 200, productResponseOf(out))
}

// Cursor is a pagination position, not authority. Bind it to the actor, tenant,
// endpoint and effective filters; SQL applies the tenant before the position.
type productPageCursor struct {
	At      time.Time `json:"at"`
	ID      uuid.UUID `json:"id"`
	Binding string    `json:"binding"`
}

func productCursorBinding(scope application.ProductScope, path string, filters any) string {
	data, _ := json.Marshal(struct {
		Scope   application.ProductScope
		Path    string
		Filters any
	}{scope, path, filters})
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
func encodeProductCursor(at time.Time, id uuid.UUID, binding string) string {
	data, _ := json.Marshal(productPageCursor{at, id, binding})
	return base64.RawURLEncoding.EncodeToString(data)
}
func decodeProductCursor(raw, binding string) (productPageCursor, error) {
	if len(raw) > 1024 {
		return productPageCursor{}, application.ErrInvalidProductQuery
	}
	data, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return productPageCursor{}, application.ErrInvalidProductQuery
	}
	var c productPageCursor
	d := json.NewDecoder(strings.NewReader(string(data)))
	d.DisallowUnknownFields()
	if d.Decode(&c) != nil || d.Decode(new(any)) != io.EOF || c.Binding != binding || c.ID == uuid.Nil || c.At.IsZero() || c.At.Year() < 1 || c.At.Year() > 9999 {
		return productPageCursor{}, application.ErrInvalidProductQuery
	}
	return c, nil
}
func productLimit(r *http.Request) (int, error) {
	raw := r.URL.Query().Get("limit")
	if raw == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 || n > application.MaxProductPageSize {
		return 0, application.ErrInvalidProductQuery
	}
	return n, nil
}

func (h *ProductHandler) List(w http.ResponseWriter, r *http.Request) {
	scope, ok := productRequestScope(w, r)
	if !ok {
		return
	}
	limit, err := productLimit(r)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	q := r.URL.Query()
	inactive := false
	if raw := q.Get("include_inactive"); raw != "" {
		if raw != "true" && raw != "false" {
			h.fail(w, r, application.ErrInvalidProductQuery)
			return
		}
		inactive = raw == "true"
	}
	search := strings.TrimSpace(q.Get("search"))
	binding := productCursorBinding(scope, "products", struct {
		Search   string
		Inactive bool
	}{search, inactive})
	input := application.ProductListInput{Scope: scope, Search: search, IncludeInactive: inactive, PageSize: limit}
	if raw := q.Get("cursor"); raw != "" {
		c, err := decodeProductCursor(raw, binding)
		if err != nil {
			h.fail(w, r, err)
			return
		}
		input.After = &application.ProductCursor{CreatedAt: c.At, ProductID: c.ID}
	}
	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()
	page, err := h.queries.List(ctx, input)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	items := make([]productResponse, 0, len(page.Items))
	for _, p := range page.Items {
		items = append(items, productResponseOf(p))
	}
	var next *string
	if page.Next != nil {
		c := encodeProductCursor(page.Next.CreatedAt, page.Next.ProductID, binding)
		next = &c
	}
	response.JSON(w, 200, struct {
		Items []productResponse `json:"items"`
		Next  *string           `json:"next_cursor"`
	}{items, next})
}

type stockMovementResponse struct {
	ID                 uuid.UUID                `json:"id"`
	ProductID          uuid.UUID                `json:"product_id"`
	ActorID            uuid.UUID                `json:"actor_id"`
	Kind               domain.StockMovementKind `json:"kind"`
	QuantityDelta      int64                    `json:"quantity_delta"`
	Reason             string                   `json:"reason"`
	CreatedAt          time.Time                `json:"created_at"`
	SaleItemID         *uuid.UUID               `json:"sale_item_id"`
	ReversesMovementID *uuid.UUID               `json:"reverses_movement_id"`
}

func (h *ProductHandler) Movements(w http.ResponseWriter, r *http.Request) {
	scope, ok := productRequestScope(w, r)
	if !ok {
		return
	}
	id, ok := productPathID(w, r)
	if !ok {
		return
	}
	limit, err := productLimit(r)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	binding := productCursorBinding(scope, "stock-movements", id)
	input := application.StockMovementListInput{Scope: scope, ProductID: id, PageSize: limit}
	if raw := r.URL.Query().Get("cursor"); raw != "" {
		c, err := decodeProductCursor(raw, binding)
		if err != nil {
			h.fail(w, r, err)
			return
		}
		input.After = &application.StockMovementCursor{CreatedAt: c.At, MovementID: c.ID}
	}
	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()
	page, err := h.queries.Movements(ctx, input)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	items := make([]stockMovementResponse, 0, len(page.Items))
	for _, m := range page.Items {
		var origin *uuid.UUID
		if m.SaleItemID != uuid.Nil {
			id := m.SaleItemID
			origin = &id
		}
		var reversed *uuid.UUID
		if m.ReversesMovementID != uuid.Nil {
			id := m.ReversesMovementID
			reversed = &id
		}
		items = append(items, stockMovementResponse{m.ID, m.ProductID, m.ActorID, m.Kind, m.QuantityDelta, m.Reason, m.CreatedAt, origin, reversed})
	}
	var next *string
	if page.Next != nil {
		c := encodeProductCursor(page.Next.CreatedAt, page.Next.MovementID, binding)
		next = &c
	}
	response.JSON(w, 200, struct {
		Items []stockMovementResponse `json:"items"`
		Next  *string                 `json:"next_cursor"`
	}{items, next})
}
func (h *ProductHandler) fail(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, application.ErrProductAccessDenied):
		response.Error(w, 403, "access_denied", "product access denied")
	case errors.Is(err, application.ErrProductNotFound):
		response.Error(w, 404, "product_not_found", "product not found")
	case errors.Is(err, application.ErrInvalidProductCommandKey):
		response.Error(w, 400, "invalid_idempotency_key", "Idempotency-Key must contain 64 lowercase hex characters")
	case errors.Is(err, application.ErrInvalidProductQuery):
		response.Error(w, 400, "invalid_product_query", "invalid product pagination or search")
	case errors.Is(err, domain.ErrInvalidProduct):
		response.Error(w, 422, "invalid_product", "invalid product name, description or positive integer price")
	case errors.Is(err, domain.ErrInvalidStock):
		response.Error(w, 422, "invalid_stock_movement", "invalid stock operation, quantity or reason")
	case errors.Is(err, domain.ErrInsufficientStock):
		response.Error(w, 409, "insufficient_stock", "not enough product units")
	case errors.Is(err, domain.ErrStockLimitExceeded):
		response.Error(w, 409, "stock_limit_exceeded", "stock quantity exceeds its limit")
	case errors.Is(err, domain.ErrProductVersionConflict), errors.Is(err, domain.ErrStockVersionConflict):
		response.Error(w, 409, "product_version_conflict", "reload product and review the current version")
	case errors.Is(err, application.ErrProductCommandConflict):
		response.Error(w, 409, "idempotency_conflict", "key was used with another input")
	case errors.Is(err, application.ErrProductCommandPending):
		response.Error(w, 409, "product_command_pending", "retry the same command after confirmation")
	default:
		h.logger.ErrorContext(r.Context(), "product request could not be completed", "request_id", chimiddleware.GetReqID(r.Context()))
		response.Error(w, 503, "temporarily_unavailable", "retry the original key and input after checking the result")
	}
}
