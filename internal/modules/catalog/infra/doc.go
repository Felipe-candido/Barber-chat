// Package infra is the boundary for concrete catalog adapters.
//
// Planned adapters include tenant-scoped SQL for professionals, services and
// assignments, and HTTP mapping for catalog administration and public reads.
// Map database rows to application/domain types without exposing pgx to them.
//
// Same-shop foreign keys protect assignments. Queries used while booking must
// participate in the reservation's transaction and catalog-validation protocol;
// an adapter must not silently open an independent transaction for that read.
//
// Receive shared connections through composition. Domain and application do
// not import this package. No SQL, routes or adapter implementations exist yet.
package infra
