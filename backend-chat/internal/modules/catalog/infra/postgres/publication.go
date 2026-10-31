package postgres

import (
	"context"
	shops "github.com/Felipe-candido/Barber-chat/internal/modules/shops/application"
	"github.com/google/uuid"
)

func (r *Repository) ReadPublicationCatalog(ctx context.Context, id uuid.UUID) (shops.PublicationCatalog, error) {
	hasService, err := r.queries.HasPublicationService(ctx, id)
	if err != nil {
		return shops.PublicationCatalog{}, err
	}
	rows, err := r.queries.ListPublicationCandidates(ctx, id)
	if err != nil {
		return shops.PublicationCatalog{}, err
	}
	result := shops.PublicationCatalog{HasService: hasService, Candidates: make([]shops.PublicationCandidate, 0, len(rows))}
	for _, row := range rows {
		result.Candidates = append(result.Candidates, shops.PublicationCandidate{ProfessionalID: row.ID, DurationMinutes: int(row.DurationMinutes)})
	}
	return result, nil
}
