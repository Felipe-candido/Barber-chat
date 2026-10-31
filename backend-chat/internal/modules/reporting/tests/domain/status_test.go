package domain_test

import (
	"testing"

	"github.com/Felipe-candido/Barber-chat/internal/modules/reporting/domain"
)

func TestReportStatusIsAValidatedReadProjection(t *testing.T) {
	for _, value := range []domain.AppointmentStatus{domain.StatusScheduled, domain.StatusConfirmed, domain.StatusCancelled, domain.StatusCompleted, domain.StatusNoShow} {
		if !value.Valid() {
			t.Fatal("known outcome rejected", value)
		}
	}
	for _, value := range []domain.AppointmentStatus{"", "paid", "done", "admin"} {
		if value.Valid() {
			t.Fatal("unknown outcome accepted", value)
		}
	}
}

func TestReportProjectsStoredProvenanceWithoutImportingBooking(t *testing.T) {
	for _, value := range []domain.AppointmentStatusSource{domain.SourceLegacy, domain.SourceBooking, domain.SourcePublic, domain.SourceStaff, domain.SourceAutomatic} {
		if !value.Valid() {
			t.Fatal("known provenance rejected", value)
		}
	}
	for _, value := range []domain.AppointmentStatusSource{"", "manual", "paid"} {
		if value.Valid() {
			t.Fatal("unknown provenance accepted", value)
		}
	}
}
