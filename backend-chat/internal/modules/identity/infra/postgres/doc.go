// Package postgres implements the application's identity read ports using sqlc
// and pgx. Repository satisfies IdentityRepository and AccessibleShopReader.
//
// NewRepository receives queries backed by a shared pool or caller-owned
// transaction. It does not create connections or manage transaction lifetimes.
// Find methods translate pgx.ErrNoRows into found=false; other failures remain
// errors. ListMembershipsByUser returns an empty, non-nil slice for no matches.
//
// ListActiveShopsByUser joins users, memberships and shops in one read query,
// requiring all three to be active. It returns application-level shop summaries
// ordered by name then shop ID, including a non-nil empty slice for no matches.
// This projection does not own shop mutations or grant lasting authorization.
//
// Mappings preserve database IDs and active flags, including suspended records.
// Domain constructors create new active entities, so they are not used to load
// existing records. Identity resolution and access decisions remain in application.
// Source queries are in db/queries/identity.sql; generated types stay in this adapter.
package postgres
