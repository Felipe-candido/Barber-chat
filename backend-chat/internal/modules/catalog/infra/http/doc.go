// Package http adapts the service catalog use cases to net/http and Chi.
//
// RegisterRoutes exposes POST /api/v1/admin/services and
// GET /api/v1/public/shops/{slug}/services. NewHandler receives the two use cases,
// an optional development shop slug, a database operation timeout and a logger.
//
// # Creation
//
// The local-only POST rejects writes unless DEV_SHOP_SLUG is configured. The
// composition/configuration requires a loopback listener; the handler checks
// the peer address and Host and rejects Origin/cross-site browser requests.
// It never uses forwarded headers as proof of identity. This development
// facility must not be exposed through a proxy and is not staff authentication.
//
// The request must be application/json, at most 64 KiB, and contain exactly one
// non-null object. Unknown fields and trailing content are rejected. Price must
// be explicitly present; zero is valid. The server-configured slug is supplied
// to CreateService. A client cannot override it through a header or JSON field.
//
// # Responses
//
// Successful creation returns 201 and a serviceResponse. Listing returns 200
// and an array, including [] for an empty catalog. Errors use
// {"error":{"code":"...","message":"..."}}. Malformed JSON returns 400,
// disabled/nonlocal writes 403, missing/inactive shops 404, oversized bodies
// 413, unsupported media 415, and domain validation 422. Context deadline or
// cancellation returns 503. Unexpected failures return a generic 500 and an
// operation-only log message; raw SQL errors and credentials are not exposed.
//
// Each request applies a context timeout before invoking a use case. Domain
// rules stay in domain and queries stay in the PostgreSQL adapter. No handler
// creates a pool, runs migrations, emits AMQP messages or builds SQL.
package http
