// Package infra is the boundary for read-only reporting adapters.
//
// Planned adapters include SQL aggregations/projections and authorized HTTP
// response mapping. Reviewed queries may join module-owned tables, but must
// always filter by tenant and must never modify records owned by other modules.
//
// Aggregate completed appointment snapshots rather than current catalog prices;
// retain currency boundaries and explicit period semantics from application.
// A SQL projection can populate a report result without loading whole aggregates.
//
// Receive shared database access from composition through internal/platform.
// Do not create a reporting database, cache or materialized view before it is
// justified. No report queries or endpoints have been implemented yet.
package infra
