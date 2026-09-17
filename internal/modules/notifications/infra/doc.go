// Package infra is the boundary for notification persistence and delivery adapters.
//
// Planned adapters include PostgreSQL jobs/outbox/attempt storage, RabbitMQ
// publication and consumption, a deterministic fake Sender, and a future provider
// client/webhook verifier. Provider DTOs and AMQP deliveries stay in this layer.
//
// Reuse shared connection setup from internal/platform. Infrastructure owns
// confirms, routing checks, ACK/NACK, leases and reconnection; application owns
// notification eligibility, retry decisions and recording delivery outcomes.
// Persist the outcome before acknowledging a delivery. Bind planning/invalidation
// writes to the booking transaction rather than committing them independently.
//
// Authenticate provider callbacks before invoking reply/result use cases. Do not
// change appointment tables directly. Domain/application do not import infra.
// There are no provider clients, broker adapters or fake senders implemented yet.
package infra
