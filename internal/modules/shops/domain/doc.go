// Package domain defines the barbershop boundary and its business vocabulary.
//
// It owns shop identity, the public slug, display information, active state and
// IANA timezone. A shop is the tenant boundary; tenant identifiers must be
// resolved from a public slug or an authorized administrative membership.
//
// Staff authorization belongs to identity. Professionals and services belong
// to catalog. Working hours, breaks, time off and calendar blocks belong to
// booking, including shop-wide scheduling defaults.
//
// This package currently documents the boundary only. Domain types and rules
// will be added with their first use case. Application workflows live in the
// sibling application package; neither layer should import HTTP, SQL or AMQP
// clients. Concrete module adapters belong to the sibling infra package.
package domain
