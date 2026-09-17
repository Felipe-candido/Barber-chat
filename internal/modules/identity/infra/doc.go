// Package infra is the boundary for administrative identity adapters.
//
// Planned adapters include verified identity-provider integration, session/HTTP
// handling and PostgreSQL membership storage. No provider or authentication
// mechanism has been selected; do not invent credentials or verification logic.
//
// Transport validates credentials and maps a verified principal to application
// inputs. Tenant membership and authorization decisions remain application/domain
// concerns. Provider-specific claims/SDK types must not leak into those layers.
//
// Receive the shared pool or transaction through composition, including the
// transaction that creates a shop and its owner membership. No adapters exist yet.
package infra
