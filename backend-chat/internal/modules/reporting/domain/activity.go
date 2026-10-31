package domain

// Origin filters select independent branches; a product filter never filters
// appointment rows, and a service filter never filters product rows in All.
type ActivitySource string

const (
	ActivityAll      ActivitySource = "all"
	ActivityServices ActivitySource = "services"
	ActivityProducts ActivitySource = "products"
)

func (s ActivitySource) Valid() bool {
	return s == ActivityAll || s == ActivityServices || s == ActivityProducts
}

func (s ActivitySource) AllowsFilters(hasService, hasProduct bool) bool {
	return s.Valid() && !(s == ActivityServices && hasProduct) && !(s == ActivityProducts && hasService)
}

// All is a query filter, never the identity of a history row/cursor.
func (s ActivitySource) IsRowSource() bool { return s == ActivityServices || s == ActivityProducts }

type ProductSaleStatus string

const (
	ProductSaleRecorded  ProductSaleStatus = "recorded"
	ProductSaleCancelled ProductSaleStatus = "cancelled"
)

func (s ProductSaleStatus) Valid() bool { return s == ProductSaleRecorded || s == ProductSaleCancelled }
