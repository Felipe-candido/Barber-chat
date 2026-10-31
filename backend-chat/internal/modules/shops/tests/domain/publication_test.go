package domain_test

import (
	"testing"

	"github.com/Felipe-candido/Barber-chat/internal/modules/shops/domain"
)

func TestPublicationRequiresAllConditions(t *testing.T) {
	for _, requested := range []bool{false, true} {
		for _, active := range []bool{false, true} {
			for _, entitled := range []bool{false, true} {
				p := domain.Publication{Requested: requested}
				if got := p.PublicBookingEnabled(active, entitled); got != (requested && active && entitled) {
					t.Fatalf("requested=%v active=%v entitled=%v: got %v", requested, active, entitled, got)
				}
			}
		}
	}
}
