// Package domain defines administrative identity and tenant authorization.
//
// It owns staff memberships, roles and access decisions for a shop. Public
// customers are not staff accounts. A catalog professional may be linked to an
// authorized user later, but these concepts are not interchangeable.
//
// Authentication proves who the user is; membership authorizes access to a shop.
// Provider identity alone must never authorize an arbitrary shop_id in a request.
//
// This package currently documents the boundary only. Authentication provider,
// sessions and administrative endpoints have not been implemented or selected.
package domain
