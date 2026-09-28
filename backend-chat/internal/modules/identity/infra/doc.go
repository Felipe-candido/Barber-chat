// Package infra is the boundary for administrative identity adapters.
//
// The postgres package reads local users and memberships through sqlc. Planned
// adapters include verified identity-provider integration and session/HTTP
// handling. Supabase Auth is selected; the
// provider-specific migration links public.users.id to auth.users.id. It is
// separate from portable migrations because plain PostgreSQL has no Auth schema.
//
// Transport validates credentials and maps a verified principal to application
// inputs. Tenant membership and authorization decisions remain application/domain
// concerns. Provider-specific claims/SDK types must not leak into those layers.
//
// Receive the shared pool or transaction through composition, including the
// transaction that provisions a profile and membership. There are no roles yet;
// SQL provisioning is manual; the read adapter does not create or grant access.
package infra
