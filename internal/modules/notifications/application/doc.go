// Package application is the home of notification use cases.
//
// Planned workflows, not implemented yet:
//   - PlanBookingNotifications: persist confirmation and reminder intents.
//   - InvalidateAppointmentNotifications: cancel stale pending jobs by version.
//   - MaterializeDueNotifications: turn persisted due jobs into outbox entries.
//   - DeliverNotification: revalidate state/consent, send and record the attempt.
//   - ScheduleRetry: persist bounded backoff after a transient failure.
//   - RecordDeliveryResult: reconcile callbacks with provider acceptance.
//   - HandleCustomerReply: deduplicate/correlate SIM or NAO and request a booking action.
//
// Planning and invalidation share the booking transaction. Delivery checks the
// current appointment version and consent; retries retain a stable logical
// notification ID. Exactly-once external delivery cannot be assumed.
//
// Provider access will use a narrow Sender port, first implemented by a fake
// adapter with deterministic success/failure scenarios. Reply handling calls a
// booking capability through a consumer-owned port, never by updating its tables.
//
// Broker ACKs, channel reconnection and outbox publication are transport concerns.
// The worker invokes these use cases, and acknowledges only after their durable
// outcome. No port or workflow is implemented before its first real consumer.
package application
