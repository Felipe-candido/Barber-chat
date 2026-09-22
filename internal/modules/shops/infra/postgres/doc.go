// Package postgres provides the concrete active-shop slug resolver.
//
// NewResolver receives generated db.Queries using the application's shared
// pool or a test transaction. LookupActiveShop executes GetActiveShopIDBySlug.
// It returns the UUID and found=true for an active shop, found=false for an
// unknown/inactive slug, and a separate error for database failures.
//
// This method satisfies catalog/application.ShopResolver structurally: Go
// interfaces do not require an implements declaration. The adapter therefore
// needs no import of the catalog domain or its error definitions. Shops owns
// identity and activity; catalog consumes the lookup through its own port.
//
// A slug is an identifier, not a secret or authorization proof. Public reads
// resolve a path slug. Local catalog writes resolve a server-configured slug.
// Future administrative writes must first verify identity/membership.
//
// This adapter does not create tenants or seed sample data. The development
// seed is run explicitly; future shop creation and initial membership must be
// coordinated atomically by their application workflow.
package postgres
