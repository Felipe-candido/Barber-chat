// Package postgres implements application.IdentityRepository using sqlc and pgx.
//
// NewRepository receives queries backed by a shared pool or caller-owned
// transaction. It does not create connections or manage transaction lifetimes.
// Find methods translate pgx.ErrNoRows into found=false; other failures remain
// errors. ListMembershipsByUser returns an empty, non-nil slice for no matches.
//
// Mappings preserve database IDs and active flags, including suspended records.
// Domain constructors create new active entities, so they are not used to load
// existing records. Identity resolution and access decisions remain in application.
// Source queries are in db/queries/identity.sql; generated types stay in this adapter.
package postgres
