// Package domain owns appointment snapshots, creation/cancellation rules and
// calendar calculations. Contacts belong to customers; services/professionals
// belong to catalog. No transport or persistence dependencies enter this layer.
//
// SlotPolicy rounds service duration plus buffer to whole configurable blocks.
// Calendar resolves civil working windows in the shop's IANA timezone, excludes
// busy intervals and past starts, and conservatively omits ambiguous/nonexistent
// DST times. Persisted appointments use UTC instants and half-open [start,end)
// reservations. A cancelled appointment releases its interval.
//
// Pure validation improves feedback; PostgreSQL exclusion constraints and shared
// transactional professional locks provide the concurrent integrity guarantee.
// Only creation and cancellation are executable transitions in this increment.
package domain
