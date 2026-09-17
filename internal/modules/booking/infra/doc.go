// Package infra is the boundary for concrete booking adapters.
//
// Planned adapters include HTTP request/response mapping and PostgreSQL access
// for availability, appointments and calendar rules. SQL enforces exclusion
// constraints, tenant references and the common agenda-locking protocol.
//
// The concrete transaction boundary must bind the ports for customer resolution,
// booking and notification intents to one transaction. Do not commit each step
// independently, expose pgx.Tx to the domain, or send messages before commit.
// Translate overlap violations into an application conflict before HTTP mapping.
//
// Generic connection setup stays in internal/platform. Handlers invoke use cases;
// they do not duplicate business decisions or query the database directly.
// Domain/application never import infra. These adapters are not implemented yet.
package infra
