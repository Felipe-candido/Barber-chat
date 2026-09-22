// Package infra is the boundary for concrete shop adapters.
//
// Planned adapters include PostgreSQL persistence for shop identity/settings
// and HTTP mapping for public slug lookup and authorized administrative changes.
// They call application workflows and implement the ports those workflows need.
//
// Receive a pool or transaction from composition; do not create a private pool
// or run migrations inside this module. Shop creation and the owner's initial
// membership must use the same transaction. Slug uniqueness belongs in the
// database as well as input validation.
//
// Reuse internal/platform for generic connections. Application and domain must
// not import infra. Add concrete adapter subpackages only with working code;
// postgres.Resolver now implements active-shop lookup by slug for catalog use
// cases. Shop CRUD and membership creation are still future workflows.
package infra
