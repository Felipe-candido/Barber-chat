package http

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/Felipe-candido/Barber-chat/internal/httpapi/requestctx"
	"github.com/Felipe-candido/Barber-chat/internal/httpapi/response"
	"github.com/Felipe-candido/Barber-chat/internal/modules/reporting/application"
	"github.com/Felipe-candido/Barber-chat/internal/modules/reporting/domain"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
)

type activityQueries interface {
	Overview(context.Context, application.ActivityReportInput) (application.ActivityOverview, error)
	History(context.Context, application.ActivityHistoryInput) (application.ActivityHistoryPage, error)
}

type Handler struct {
	queries activityQueries
	timeout time.Duration
	logger  *slog.Logger
}

func NewHandler(queries activityQueries, timeout time.Duration, logger *slog.Logger) *Handler {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Handler{queries: queries, timeout: timeout, logger: logger}
}

func (h *Handler) Overview(w http.ResponseWriter, r *http.Request) {
	in, _, ok := reportInput(w, r, false)
	if !ok {
		return
	}
	if h.queries == nil {
		h.fail(w, r, errors.New("report queries unavailable"))
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()
	out, err := h.queries.Overview(ctx, in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	body, err := overviewBody(out)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, http.StatusOK, body)
}

func (h *Handler) History(w http.ResponseWriter, r *http.Request) {
	common, values, ok := reportInput(w, r, true)
	if !ok {
		return
	}
	in := application.ActivityHistoryInput{ActivityReportInput: common,
		AppointmentStatus: domain.AppointmentStatus(values.Get("appointment_status")), SaleStatus: domain.ProductSaleStatus(values.Get("sale_status"))}
	if values.Has("limit") {
		limit, err := strconv.Atoi(values.Get("limit"))
		if err != nil || limit < 1 || limit > application.MaxHistoryPageSize {
			h.fail(w, r, application.ErrInvalidQuery)
			return
		}
		in.PageSize = limit
	}
	if values.Has("cursor") {
		cursor, err := decodeCursor(values.Get("cursor"))
		if err != nil {
			h.fail(w, r, err)
			return
		}
		in.After = cursor
	}
	if h.queries == nil {
		h.fail(w, r, errors.New("report queries unavailable"))
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()
	out, err := h.queries.History(ctx, in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	body, err := historyBody(out)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, http.StatusOK, body)
}

func reportInput(w http.ResponseWriter, r *http.Request, history bool) (application.ActivityReportInput, url.Values, bool) {
	auth, authenticated := requestctx.StaffIdentityFromContext(r.Context())
	scope, authorized := requestctx.ShopScopeFromContext(r.Context())
	if !authenticated || auth.UserID == uuid.Nil {
		w.Header().Set("WWW-Authenticate", `Bearer realm="barber-chat"`)
		response.Error(w, 401, "authentication_required", "authentication is required")
		return application.ActivityReportInput{}, nil, false
	}
	if !authorized || scope.ShopID == uuid.Nil || scope.UserID != auth.UserID || !scope.Role.Valid() || (scope.Role == "barber" && scope.ProfessionalID == uuid.Nil) {
		response.Error(w, 403, "access_denied", "shop access is required")
		return application.ActivityReportInput{}, nil, false
	}
	// Bound parsing work before allocating decoded values, including the opaque cursor.
	if len(r.URL.RawQuery) > 4096 {
		response.Error(w, 400, "invalid_report_query", "review dates, filters, limit and cursor")
		return application.ActivityReportInput{}, nil, false
	}
	v, err := url.ParseQuery(r.URL.RawQuery)
	if err == nil {
		for key, items := range v {
			if len(items) != 1 || items[0] == "" {
				err = application.ErrInvalidQuery
				break
			}
			switch key {
			case "from", "through", "source", "customer_name", "phone", "phone_country", "professional_id", "service_id", "product_id":
			case "appointment_status", "sale_status", "limit", "cursor":
				if !history {
					err = application.ErrInvalidQuery
				}
			default:
				err = application.ErrInvalidQuery
			}
		}
	}
	if err != nil || !v.Has("from") || !v.Has("through") {
		response.Error(w, 400, "invalid_report_query", "review dates, filters, limit and cursor")
		return application.ActivityReportInput{}, nil, false
	}
	in := application.ActivityReportInput{ActorID: auth.UserID, ShopID: scope.ShopID, FromDate: v.Get("from"), ThroughDate: v.Get("through"), Source: domain.ActivitySource(v.Get("source")),
		Filters: application.ActivityFilterInput{ReportFilterInput: application.ReportFilterInput{CustomerName: v.Get("customer_name"), Phone: v.Get("phone"), PhoneCountry: v.Get("phone_country")}}}
	for key, target := range map[string]*uuid.UUID{"professional_id": &in.Filters.ProfessionalID, "service_id": &in.Filters.ServiceID, "product_id": &in.Filters.ProductID} {
		if !v.Has(key) {
			continue
		}
		id, err := uuid.Parse(v.Get(key))
		if err != nil || id == uuid.Nil {
			response.Error(w, 400, "invalid_report_query", "review dates, filters, limit and cursor")
			return application.ActivityReportInput{}, nil, false
		}
		*target = id
	}
	return in, v, true
}

func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, application.ErrInvalidQuery):
		response.Error(w, 400, "invalid_report_query", "review dates, filters, limit and cursor")
	case errors.Is(err, application.ErrActivityAccessDenied):
		response.Error(w, 403, "access_denied", "shop access is required")
	case errors.Is(err, application.ErrShopNotFound):
		response.Error(w, 404, "shop_not_found", "shop not found")
	case errors.Is(err, application.ErrInconsistentData):
		h.logger.ErrorContext(r.Context(), "report projection could not be validated", "request_id", chimiddleware.GetReqID(r.Context()))
		response.Error(w, 500, "report_unavailable", "report could not be loaded")
	default:
		// Do not log raw errors, query strings, contacts, cursors or tokens.
		h.logger.WarnContext(r.Context(), "report read could not be completed", "request_id", chimiddleware.GetReqID(r.Context()))
		response.Error(w, 503, "temporarily_unavailable", "report could not be loaded; retry the read")
	}
}
