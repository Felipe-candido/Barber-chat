package application

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"strings"
	"time"

	"github.com/Felipe-candido/Barber-chat/internal/modules/reporting/domain"
	"github.com/google/uuid"
)

// Protocol 1 binds a page position to normalized filters and CURRENT authority.
// Page size and observation time are intentionally excluded: size may change
// within its limit, and each page is a fresh authorized database snapshot.
// This hash is not a signature or an access credential. HTTP encoding is separate.
func activityFingerprint(actor uuid.UUID, a ActivityAudience, q ActivityHistoryQuery) ([32]byte, error) {
	canonical := struct {
		Protocol                        int
		ActorID, ShopID, ProfessionalID uuid.UUID
		CanReadAll                      bool
		From, Until                     time.Time
		Timezone                        string
		Source                          domain.ActivitySource
		CustomerName, PhoneE164         string
		ServiceID, ProductID            uuid.UUID
		AppointmentStatus               domain.AppointmentStatus
		SaleStatus                      domain.ProductSaleStatus
	}{1, actor, q.ShopID, q.Filters.ProfessionalID, a.CanReadAll, q.Period.From(), q.Period.Until(), q.Period.Timezone(), q.Source,
		q.Filters.CustomerName, q.Filters.PhoneE164, q.Filters.ServiceID, q.Filters.ProductID, q.AppointmentStatus, q.SaleStatus}
	data, err := json.Marshal(canonical)
	if err != nil {
		return [32]byte{}, ErrInconsistentData
	}
	return sha256.Sum256(data), nil
}

func validCursor(q ActivityHistoryQuery, c ActivityCursor) bool {
	return c.Source.IsRowSource() && c.RecordID != uuid.Nil && !c.OccurredAt.Before(q.Period.From()) && c.OccurredAt.Before(q.Period.Until()) &&
		(q.Source == domain.ActivityAll || q.Source == c.Source) &&
		((c.Source == domain.ActivityServices && c.ItemID == uuid.Nil) || (c.Source == domain.ActivityProducts && c.ItemID != uuid.Nil))
}

func activityPosition(r ActivityHistoryRow) ActivityCursor {
	return ActivityCursor{OccurredAt: r.OccurredAt.UTC(), Source: r.Source, RecordID: r.RecordID, ItemID: r.ItemID}
}

// Match SQL's C collation and UUID byte ordering, independent of locale.
func comparePositions(a, b ActivityCursor) int {
	if c := a.OccurredAt.Compare(b.OccurredAt); c != 0 {
		return c
	}
	if c := strings.Compare(string(a.Source), string(b.Source)); c != 0 {
		return c
	}
	if c := bytes.Compare(a.RecordID[:], b.RecordID[:]); c != 0 {
		return c
	}
	return bytes.Compare(a.ItemID[:], b.ItemID[:])
}
