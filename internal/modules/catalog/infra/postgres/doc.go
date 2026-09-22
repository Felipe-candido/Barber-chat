// Package postgres implements application.ServiceRepository using sqlc and pgx.
//
// NewRepository receives db.Queries backed by the shared pool or a caller-owned
// transaction. It does not create/close pools or start independent transactions.
// cmd/api supplies the pool-backed queries; integration tests supply tx-backed
// queries and roll back their fixtures.
//
// Create maps a validated domain.Service into generated query parameters.
// Duration is converted to int32 after domain validation has checked its range.
// INSERT ... SELECT only inserts for the supplied active shop ID and returns
// the persisted row. No matching active shop maps to application.ErrShopNotFound.
// Other driver errors remain internal until the HTTP adapter sanitizes them.
//
// ListActiveByShop filters by shop_id and service.active and joins shops to
// require an active shop. Ordering by name then ID is stable for duplicate names.
// Both operations map rows through toDomain; pgx timestamp wrappers and generated
// database types do not escape into the application/domain layers.
//
// Source queries live in db/queries/catalog.sql. Constraints and currency are
// maintained through versioned migrations. The present operations each use one
// SQL statement. Future booking reads must use the reservation transaction
// supplied by composition rather than opening a separate pool or transaction.
package postgres
