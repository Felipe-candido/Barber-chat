// Package domain defines the professional and service catalog boundary.
//
// It owns professionals, services and professional-service assignments within
// a shop. Services include name, description, duration, price, currency and
// active state. A professional is not necessarily an authenticated staff user.
//
// Booking owns appointment snapshots and calendar occupancy. Catalog changes
// must not rewrite historical appointment prices, names or durations. Completed
// work is reported from appointments; there is no portfolio entity.
//
// This package currently documents the boundary only. Add domain types with
// their first use case, without HTTP, persistence or message-provider imports.
package domain
