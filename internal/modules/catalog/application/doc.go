// Package application is the home of catalog use cases.
//
// Planned workflows, not implemented yet:
//   - CreateProfessional and UpdateProfessional: manage shop professionals.
//   - CreateService and UpdateService: manage descriptions, prices and durations.
//   - SetServiceActive: control whether a service can be booked.
//   - AssignService and UnassignService: manage professional capabilities.
//   - ListBookableServices and ListServiceProfessionals: public catalog reads.
//
// Every operation is tenant-scoped. Writes must preserve same-shop references;
// disabling a service does not silently cancel existing appointments. Price
// overrides per professional remain a product decision, not an assumed feature.
//
// Booking reads effective service/professional data through its own consumer
// port and validates it during reservation. Keep provider-specific DTOs and
// pgx types out of workflow inputs and outputs.
//
// Add action-named files as each workflow is implemented; no CRUD is wired yet.
package application
