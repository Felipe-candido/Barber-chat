// Package domain defines administrative identity and tenant authorization.
//
// It owns administrative memberships and access decisions for a shop. Public
// customers are not staff accounts. A catalog professional may be linked to an
// authorized user later, but these concepts are not interchangeable.
//
// Authentication proves who the user is; membership authorizes access to a shop.
// Provider identity alone must never authorize an arbitrary shop_id in a request.
//
// Supabase Auth is selected. The initial database model has users and memberships
// without roles; all eligible members will have equal access within their shop.
// A user's ID is the provider UUID. See migrations and ADR 0009.
// User and Membership model the initial identifier and active-state invariants.
// Persistence is implemented in infra/postgres and token verification in
// infra/supabase. The HTTP boundary exposes the verified local identity;
// sessions and HTTP endpoints belong outside this package.
package domain
