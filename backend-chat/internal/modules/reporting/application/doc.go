// Package application is the home of reporting queries.
//
// Planned workflows, not implemented yet:
//   - GetAppointmentSummary: counts by completed, cancelled and no_show outcome.
//   - GetCompletedServiceRevenue: aggregate completed snapshot values by period.
//   - GetRevenueByService and GetRevenueByProfessional: grouped snapshot totals.
//   - ListCompletedAppointments: list the actual services performed.
//
// Queries require an authorized tenant scope and explicit period semantics in
// the shop's timezone, converted into a half-open interval of instants. Exclude
// other tenants and keep currencies separate when calculating totals.
//
// Read adapters may use reviewed joins across module-owned tables. They must not
// mutate booking/catalog records or recalculate historical prices from today's
// catalog. Read models need not instantiate entire booking aggregates.
//
// Period anchor and correction policy must be settled with the first report.
// There are no queries, materialized views or report endpoints in this scaffold.
package application
