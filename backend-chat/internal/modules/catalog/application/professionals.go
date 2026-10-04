package application

import (
	"context"
	"sort"

	"github.com/Felipe-candido/Barber-chat/internal/modules/catalog/domain"
	"github.com/google/uuid"
)

type CreateProfessional struct{ transactions ProfessionalTransactor }

func NewCreateProfessional(tx ProfessionalTransactor) *CreateProfessional {
	return &CreateProfessional{tx}
}

func (uc *CreateProfessional) Execute(ctx context.Context, input CreateProfessionalInput) (ProfessionalOutput, error) {
	professional, err := domain.NewProfessional(input.ShopID, input.Name)
	if err != nil {
		return ProfessionalOutput{}, err
	}
	if len(input.ServiceIDs) == 0 || len(input.ServiceIDs) > 100 {
		return ProfessionalOutput{}, ErrInvalidProfessionalServices
	}
	services := append([]uuid.UUID(nil), input.ServiceIDs...)
	sort.Slice(services, func(i, j int) bool { return services[i].String() < services[j].String() })
	for i, id := range services {
		if id == uuid.Nil || (i > 0 && services[i-1] == id) {
			return ProfessionalOutput{}, ErrInvalidProfessionalServices
		}
	}
	input.Schedule.ShopID = professional.ShopID
	input.Schedule.ProfessionalID = professional.ID
	err = uc.transactions.WithinProfessionalTx(ctx, func(ctx context.Context, tx ProfessionalTx) error {
		if err := tx.Professionals.CreateProfessional(ctx, professional, services); err != nil {
			return err
		}
		return tx.Schedule.ConfigureInitialSchedule(ctx, input.Schedule)
	})
	if err != nil {
		return ProfessionalOutput{}, err
	}
	return ProfessionalOutput{ID: professional.ID, Name: professional.Name, Active: professional.Active, ServiceIDs: services}, nil
}

type ListProfessionals struct{ repository ProfessionalRepository }

func NewListProfessionals(repository ProfessionalRepository) *ListProfessionals {
	return &ListProfessionals{repository}
}
func (uc *ListProfessionals) Execute(ctx context.Context, shopID uuid.UUID) ([]ProfessionalOutput, error) {
	if shopID == uuid.Nil {
		return nil, domain.ErrInvalidShopID
	}
	items, err := uc.repository.ListProfessionals(ctx, shopID)
	if err != nil {
		return nil, err
	}
	if items == nil {
		items = make([]ProfessionalOutput, 0)
	}
	return items, nil
}
