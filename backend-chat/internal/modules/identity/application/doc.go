// Package application is the home of administrative access use cases.
//
// Planned workflows, not implemented yet:
//   - ResolveStaffIdentity: map a verified external identity to a local user.
//   - AuthorizeShopAction: validate membership and permission for the tenant.
//   - GrantMembership and RevokeMembership: manage authorized staff access.
//   - ChangeMembershipRole: change permissions with an auditable actor.
//
// Authentication integration and session handling will be adapters around an
// explicitly chosen mechanism. Do not introduce a home-grown password or JWT
// system merely to fill this package. Token verification must precede identity
// resolution, and authorization remains enforced in the backend.
//
// First-owner membership creation coordinates with shop creation atomically.
// Other modules receive a validated actor/tenant scope, not provider SDK types.
// This package contains a use-case map only; no administrative route is active.
package application
