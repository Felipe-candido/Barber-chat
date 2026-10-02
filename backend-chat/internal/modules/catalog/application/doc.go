// Package application coordinates catalog use cases through small ports.
//
// # CreateService
//
// NewCreateService receives a ServiceRepository implementation. Execute takes
// a ShopID from successful authorization and service input, calls domain.NewService,
// persists the result, and returns ServiceOutput. It does not read HTTP context
// values or resolve the tenant again. The caller must never trust request JSON
// as proof of shop access; the HTTP boundary supplies the authorized scope.
//
// Persistence rechecks shop activity and can return ErrShopNotFound. Invalid input returns
// a sentinel validation error without calling Create. Persistence failures
// propagate to the HTTP adapter, which sanitizes them before responding.
// The context carries cancellation/deadlines into persistence.
//
// # ListServices
//
// NewListServices receives ServiceRepository and ShopResolver. Execute resolves the public slug
// and calls ListActiveByShop with the resulting ID. Only public catalog fields
// are mapped to ServiceOutput. Empty results are non-nil slices so HTTP can
// encode an empty JSON array. Unknown/inactive shops differ from empty catalogs.
//
// # ListAuthorizedServices
//
// Execute receives a non-empty ShopID from successful authorization and queries
// ServiceRepository directly. Unlike public ListServices, it does not resolve a
// client slug again. HTTP must authenticate and authorize before invoking it.
// It currently lists active services only; inactive management is future work.
//
// # Ports and output
//
// ServiceRepository is the persistence capability used by these workflows.
// ShopResolver is a consumer-owned read port implemented in shops/infra/postgres;
// its found flag distinguishes absence from a storage error without importing
// another module's domain or a driver error. Resolution is not authorization.
// ServiceOutput keeps pgx models and HTTP JSON conventions out of use cases.
//
// Domain and application never import infra. cmd/api supplies concrete adapters.
// Future update/activation and professional workflows belong here when added;
// booking must coordinate its own shared transaction for reservation snapshots.
package application
