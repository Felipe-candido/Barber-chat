// Package application coordinates catalog use cases through small ports.
//
// # CreateService
//
// NewCreateService receives ServiceRepository and ShopResolver implementations.
// Execute takes a trusted ShopSlug and service input, resolves an active shop,
// calls domain.NewService, persists the result, and returns ServiceOutput.
// The slug must be selected by the administrative boundary. In local testing
// it comes from DEV_SHOP_SLUG; future production access must derive it or its
// tenant scope from authenticated membership, never arbitrary request JSON.
//
// A missing/inactive shop returns ErrShopNotFound. Invalid domain input returns
// a sentinel validation error without calling Create. Persistence failures
// propagate to the HTTP adapter, which sanitizes them before responding.
// The context carries cancellation/deadlines into lookup and persistence.
//
// # ListServices
//
// NewListServices receives the same two ports. Execute resolves the public slug
// and calls ListActiveByShop with the resulting ID. Only public catalog fields
// are mapped to ServiceOutput. Empty results are non-nil slices so HTTP can
// encode an empty JSON array. Unknown/inactive shops differ from empty catalogs.
//
// # Ports and output
//
// ServiceRepository is the persistence capability used by these two workflows.
// ShopResolver is a consumer-owned read port implemented in shops/infra/postgres;
// its found flag distinguishes absence from a storage error without importing
// another module's domain or a driver error. Resolution is not authorization.
// ServiceOutput keeps pgx models and HTTP JSON conventions out of use cases.
//
// Domain and application never import infra. cmd/api supplies concrete adapters.
// Future update/activation and professional workflows belong here when added;
// booking must coordinate its own shared transaction for reservation snapshots.
package application
