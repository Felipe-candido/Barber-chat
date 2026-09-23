// Package application is the home of appointment and availability use cases.
//
// Planned workflows, not implemented yet:
//   - ListAvailability: compute candidate slots for a service and professional.
//   - CreateAppointment: confirm the public form and atomically persist a booking.
//   - ConfirmAppointment and CancelAppointment: apply authorized state changes.
//   - CompleteAppointment and MarkNoShow: record an administrative outcome.
//   - ListAppointments and GetAppointment: tenant-scoped calendar reads.
//   - SetWorkingHours and SetBreaks: configure recurring calendar rules.
//   - AddTimeOff, AddCalendarBlock and RemoveCalendarBlock: manage exceptions.
//
// CreateAppointment is the first implementation target. Its transaction must
// cover request idempotency, agenda locking, catalog revalidation, customer
// resolution, consent, appointment snapshots and notification intents/jobs.
// Consumer-owned ports must share that transaction through concrete adapters;
// independently committing each module would break the booking guarantee.
//
// ListAvailability does not reserve a slot. Concurrent changes to working hours,
// blocks and appointments must follow the same locking protocol. Translate
// database overlap conflicts at the adapter boundary, not through pgx imports.
//
// Cancellation invalidates pending reminders in the same transaction. Public
// actions require a verified scoped token; a phone number or appointment ID
// alone is not authorization. Never call a message provider inside this layer.
//
// Add files such as create_appointment.go with real behavior and tests. There
// are no stub handlers, repository interfaces or callable use cases yet.
package application
