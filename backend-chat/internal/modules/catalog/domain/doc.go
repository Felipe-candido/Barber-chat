// Package domain contains the service catalog's business types and invariants.
//
// Service represents a service owned by one shop. It contains UUIDs, name,
// description, duration in minutes, integer price cents, currency and active
// state. These are business values, independent of JSON tags and pgx types.
//
// NewService trims name/description, requires a nonzero shop ID, requires a name
// of 1-100 Unicode characters, accepts a positive duration representable by the
// PostgreSQL integer column, and rejects negative prices. New services receive
// a generated UUID, active=true and currency=BRL. The duration upper bound
// protects the int-to-int32 conversion; it is not a scheduling product policy.
//
// Exported sentinel errors let the application and HTTP adapter recognize
// invalid input without examining error strings. Database constraints remain a
// second line of defense. Public fields allow mapping persisted records without
// generating a new ID; creation workflows must go through NewService.
//
// This package does not resolve slugs, authenticate staff, execute SQL, decode
// HTTP or send messages. The caller provides an already resolved shop ID.
// Shops owns that identity; booking will own appointment snapshots. Future
// changes to price, duration or name must leave historical snapshots intact.
//
// Future catalog concepts include professionals and service assignments.
// This module has no portfolio/gallery entity.
package domain
