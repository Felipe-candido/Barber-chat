package http

import "github.com/Felipe-candido/Barber-chat/internal/modules/shops/domain"

type addressJSON struct {
	PostalCode string `json:"postal_code"`
	Street     string `json:"street"`
	Number     string `json:"number"`
	Complement string `json:"complement"`
	District   string `json:"district"`
	City       string `json:"city"`
	State      string `json:"state"`
}

func (a addressJSON) domainAddress() domain.Address {
	return domain.Address{PostalCode: a.PostalCode, Street: a.Street, Number: a.Number,
		Complement: a.Complement, District: a.District, City: a.City, State: a.State}
}

func addressResponse(a *domain.Address) *addressJSON {
	if a == nil {
		return nil
	}
	return &addressJSON{PostalCode: a.PostalCode, Street: a.Street, Number: a.Number,
		Complement: a.Complement, District: a.District, City: a.City, State: a.State}
}
