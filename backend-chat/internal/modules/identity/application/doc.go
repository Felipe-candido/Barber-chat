// Package application is the home of administrative access use cases.
//
// Planned workflows, not implemented yet:
//   - ResolveStaffIdentity: map a verified external identity to a local user.
//   - AuthorizeShopAction: validate active user, membership and shop for the tenant.
//   - ListMyShops: list eligible shops without choosing an arbitrary one.
//
// Authentication integration and session handling will be adapters around an
// explicit Supabase Auth integration. Do not introduce a home-grown password or JWT
// system merely to fill this package. Token verification must precede identity
// resolution, and authorization remains enforced in the backend.
//
// Accounts and memberships are provisioned manually; no roles or CRUD are planned
// for the first increment. Profile and membership provisioning is transactional.
// Other modules receive a validated actor/tenant scope, not provider SDK types.
// This package contains a use-case map only; no administrative route is active.
package application
