// Package infra is the boundary for concrete customer adapters.
//
// Planned adapters include tenant-scoped contact/consent persistence and
// authorized administrative HTTP reads. Phone metadata validation may use a
// concrete library here when required by an application-owned port.
//
// Find-or-create operations during booking must use the provided transaction,
// preserve same-tenant uniqueness and avoid independent commits. Map constraint
// errors to application errors; never expose DSNs, raw SQL or customer PII.
//
// No public incomplete-customer registration endpoint is planned. Pools come
// from composition via internal/platform. No adapter is implemented yet.
package infra
