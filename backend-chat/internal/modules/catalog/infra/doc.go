// Package infra documents the service catalog module and its end-to-end flow.
//
// # Scope
//
// Catalog owns service definitions and, in future increments, professionals and
// professional-service assignments. Shops owns tenant identity and slug lookup;
// identity owns administrative membership; booking owns appointment snapshots
// and availability. Editing a future service must not rewrite booking history.
//
// # Request flow
//
// cmd/api creates one PostgreSQL pool, db.Queries, the catalog repository and the
// shops resolver. It injects these adapters into NewCreateService and
// NewListServices and NewListAuthorizedServices, then injects the use cases into the HTTP handler.
// internal/httpapi registers the handler routes alongside health and readiness.
//
// A POST to /api/v1/admin/shops/{slug}/services follows this sequence:
//  1. Shared middleware authenticates the Bearer token and authorizes the exact shop.
//  2. It accepts one bounded JSON object and rejects unknown fields, including
//     shop_id and shop_slug. The handler requires matching identity/scope values.
//  3. CreateService receives the authorized ShopID explicitly.
//  4. domain.NewService normalizes and validates the service and creates its ID.
//  5. ServiceRepository.Create persists it through the PostgreSQL adapter and
//     sqlc. The INSERT also checks that the shop is still active.
//  6. The use case maps the result to ServiceOutput; HTTP returns JSON and 201.
//
// A GET to /api/v1/public/shops/{slug}/services follows a similar read path:
// the path slug identifies a public shop, ListServices resolves its ID, and the
// repository returns only active services belonging to an active shop. An
// unknown or inactive shop returns 404; an active shop with no services returns [].
//
// # Tenant and administrative access
//
// Resolving a slug identifies a tenant; it does not authenticate a user.
// Every administrative read and write requires a verified active user and an active
// membership in the active shop. GET /api/v1/admin/shops/{slug}/services passes the
// authorized ShopID to ListAuthorizedServices without resolving the slug again.
// There is no fixed-shop administrative endpoint or setting.
// Public reads need no administrative identity. No client-provided ID selects
// the target of a write. Request context is an HTTP boundary, not business state.
//
// # Boundaries
//
// Domain depends on Go and UUID types, not HTTP or PostgreSQL. Application uses
// domain and small consumer-owned ports. Infra implements those ports and maps
// transport/storage representations. Concrete dependencies are wired in cmd;
// no module opens another database pool. internal/database/sqlc is generated
// code, used only by adapters and composition, never by domain/application.
//
// # Persistence and tests
//
// Migrations are applied explicitly with Goose, outside API/worker startup.
// db/seeds/development.sql creates an optional example shop separately.
// New services store integer cents and BRL; zero price is allowed. A single
// INSERT is atomic and requires no application-level multi-statement transaction.
// Unit tests cover rules, orchestration and HTTP. Integration tests use a real
// PostgreSQL transaction and roll back their own fixtures, checking persistence,
// inactive records and isolation between two shops.
//
// Creating/listing services is implemented. Editing, activation endpoints,
// professionals and assignments remain future work.
// RabbitMQ, notifications and the worker do not participate in these requests.
package infra
