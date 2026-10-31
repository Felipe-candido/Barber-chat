package domain_test

import (
	"testing"

	"github.com/Felipe-candido/Barber-chat/internal/modules/reporting/domain"
)

func TestActivitySourceFiltersAndRowIdentity(t *testing.T) {
	for _, tc := range []struct {
		source                 domain.ActivitySource
		service, product, want bool
	}{
		{domain.ActivityAll, false, false, true}, {domain.ActivityAll, true, true, true},
		{domain.ActivityServices, true, false, true}, {domain.ActivityServices, false, true, false},
		{domain.ActivityProducts, false, true, true}, {domain.ActivityProducts, true, false, false},
		{"unknown", false, false, false}, {"", false, false, false},
	} {
		if tc.source.AllowsFilters(tc.service, tc.product) != tc.want {
			t.Fatal("incorrect branch filter", tc)
		}
	}
	if domain.ActivityAll.IsRowSource() || !domain.ActivityServices.IsRowSource() || !domain.ActivityProducts.IsRowSource() {
		t.Fatal("query source used as row identity")
	}
	if !domain.ProductSaleRecorded.Valid() || !domain.ProductSaleCancelled.Valid() || domain.ProductSaleStatus("paid").Valid() {
		t.Fatal("sale outcome mapping changed")
	}
}
