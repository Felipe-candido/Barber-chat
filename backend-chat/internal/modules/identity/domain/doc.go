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
// This package documents the boundary only; token verification, active-state
// checks, sessions and administrative identity endpoints are not implemented.
package domain
