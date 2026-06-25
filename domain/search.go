package domain

type SearchParams struct {
	Keyword         string
	ExcludeKeyword  string
	Sort            string
	Order           string
	Status          []string
	PriceMin        int
	PriceMax        int
	CategoryID      []string
	BrandID         []string
	ItemConditionID []int
	ShippingPayerID []int
	SizeID          []int
	ColorID         []int
	ItemID          string
}

func (p SearchParams) DisplayName() string {
	if p.ItemID != "" {
		return "item:" + p.ItemID
	}
	return p.Keyword
}

func KeywordSearch(keyword string) SearchParams {
	return SearchParams{
		Keyword: keyword,
		Sort:    "SORT_CREATED_TIME",
		Order:   "ORDER_DESC",
		Status:  []string{"STATUS_ON_SALE"},
	}
}
