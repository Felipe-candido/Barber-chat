// Package http adapts the service catalog use cases to net/http and Chi.
//
// RegisterPublicRoutes exposes GET /api/v1/public/shops/{slug}/services.
// internal/httpapi registers administrative GET/POST routes with authentication and
// shop authorization. NewHandler receives three use cases, a timeout and a logger.
//
// # Creation
//
// Authentication stores an active local identity in requestctx; ShopAccess
// stores the exact authorized user/shop scope. CreateService requires both and
// matching user IDs. Neither Origin, Host nor forwarded headers prove identity.
// Shared CORS and unauthenticated OPTIONS are handled by internal/httpapi.
// ListShopServices requires the same consistent identity/scope and invokes
// ListAuthorizedServices with the authorized ShopID, never a client identifier.
//
// The request must be application/json, at most 64 KiB, and contain exactly one
// non-null object. Unknown fields and trailing content are rejected. Price must
// be explicitly present; zero is valid. Only the authorized ShopID is supplied
// to the use case. A client cannot override it through a header or JSON field.
//
// # Responses
//
// Successful creation returns 201 and a serviceResponse. Listing returns 200
// and an array, including [] for an empty catalog. Errors use
// {"error":{"code":"...","message":"..."}}. Malformed JSON returns 400,
// missing authentication 401, missing/mismatched scope 403, shops deactivated
// before persistence 404, oversized bodies
// 413, unsupported media 415, and domain validation 422. Context deadline or
// cancellation returns 503. Unexpected failures return a generic 500 and an
// operation-only log message; raw SQL errors and credentials are not exposed.
//
// Each request applies a context timeout before invoking a use case. Domain
// rules stay in domain and queries stay in the PostgreSQL adapter. No handler
// creates a pool, runs migrations, emits AMQP messages or builds SQL.
package http
