// Package domain defines read-only business report concepts for a shop.
//
// Reports derive from appointment outcomes and booking-time snapshots. It does
// not own a second completed-services ledger or a portfolio/gallery entity.
// Completed service revenue is not the same as collected cash without a payment
// model, which is outside the current scope.
//
// This package documents the boundary only. Introduce report result types when
// queries exist; a reporting module does not require artificial domain entities.
package domain
