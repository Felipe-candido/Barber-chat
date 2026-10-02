// Package application is the home of administrative access use cases.
//
// Authenticate verifies an access token through a port and resolves an active
// local user. Its result identifies the user but grants no shop access.
// AuthorizeShopAction rechecks the active user, resolves an active shop and
// requires an active membership matching both IDs before returning a shop scope.
//
// ListMyShops rechecks the local user and requests eligible shop summaries through
// AccessibleShopReader without choosing a shop or granting lasting authorization.
// GET /api/v1/admin/shops supplies its UserID from the authenticated request context.
//
// Token verification is an adapter around Supabase Auth. The HTTP boundary
// authenticates Bearer requests; the frontend owns login and refresh. Do not introduce a home-grown password or JWT
// system merely to fill this package. Token verification must precede identity
// resolution, and authorization remains enforced in the backend.
//
// Accounts and memberships are provisioned manually; no roles or CRUD are planned
// for the first increment. Profile and membership provisioning is transactional.
// Other modules receive a validated actor/tenant scope, not provider SDK types.
// A PostgreSQL read adapter implements IdentityRepository. Concrete token
// verification is implemented in infra/supabase. Shared HTTP middleware invokes
// these use cases before administrative identity and catalog handlers.
//
// Transport adapters should map ErrInvalidAccessToken/ErrInvalidStaffIdentity to
// unauthenticated access and ErrUserNotProvisioned/ErrUserInactive/
// ErrShopAccessDenied to forbidden access. Operational
// errors and ErrIdentityMismatch are failures, not invalid credentials. HTTP
// status selection and safe client error messages belong to those adapters.
package application
