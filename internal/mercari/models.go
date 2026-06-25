package mercari

import "encoding/json"

type searchRequest struct {
	SearchSessionID      string          `json:"searchSessionId"`
	SearchCondition      searchCondition `json:"searchCondition"`
	PageSize             int             `json:"pageSize"`
	PageToken            string          `json:"pageToken"`
	IndexRouting         string          `json:"indexRouting"`
	ThumbnailTypes       []string        `json:"thumbnailTypes"`
	Source               string          `json:"source"`
	WithItemBrand        bool            `json:"withItemBrand"`
	WithItemPromotions   bool            `json:"withItemPromotions"`
	WithItemSizes        bool            `json:"withItemSizes"`
	WithSuggestedItems   bool            `json:"withSuggestedItems"`
	WithOfferPricePromo  bool            `json:"withOfferPricePromotion"`
	WithProductSuggest   bool            `json:"withProductSuggest"`
	WithAuction          bool            `json:"withAuction"`
	UseDynamicAttribute  bool            `json:"useDynamicAttribute"`
}

type searchCondition struct {
	Keyword         string   `json:"keyword"`
	ExcludeKeyword  string   `json:"excludeKeyword"`
	Sort            string   `json:"sort"`
	Order           string   `json:"order"`
	Status          []string `json:"status"`
	PriceMin        int      `json:"priceMin"`
	PriceMax        int      `json:"priceMax"`
	CategoryID      []string `json:"categoryId"`
	BrandID         []string `json:"brandId"`
	ItemConditionID []int    `json:"itemConditionId"`
	ShippingPayerID []int    `json:"shippingPayerId"`
	SizeID          []int    `json:"sizeId"`
	ColorID         []int    `json:"colorId"`
	HasCoupon       bool     `json:"hasCoupon"`
	ItemTypes       []string `json:"itemTypes"`
}

type searchResponse struct {
	Items []apiItem `json:"items"`
}

// json is used by itemDetail's json.Number field.
var _ = json.Number("")

type apiItem struct {
	ID              string   `json:"id"`
	SellerID        string   `json:"sellerId"`
	Status          string   `json:"status"`
	Name            string   `json:"name"`
	Price           string   `json:"price"`
	Created         string   `json:"created"`
	Updated         string   `json:"updated"`
	Thumbnails      []string `json:"thumbnails"`
	ItemConditionID string   `json:"itemConditionId"`
	ShippingPayerID string   `json:"shippingPayerId"`
	CategoryID      string   `json:"categoryId"`
	Photos          []photo  `json:"photos"`
}

type photo struct {
	URI string `json:"uri"`
}

type itemDetailResponse struct {
	Data itemDetail `json:"data"`
}

type itemDetail struct {
	ID            string          `json:"id"`
	Name          string          `json:"name"`
	Description   string          `json:"description"`
	Price         json.Number     `json:"price"`
	Status        string          `json:"status"`
	Photos        []string        `json:"photos"`
	Thumbnails    []string        `json:"thumbnails"`
	Created       json.Number     `json:"created"`
	Updated       json.Number     `json:"updated"`
	NumLikes      int             `json:"num_likes"`
	Seller        itemSeller      `json:"seller"`
	ItemCondition itemCondition   `json:"item_condition"`
	ShippingPayer itemShipPayer   `json:"shipping_payer"`
	ItemCategory  itemCategory    `json:"item_category"`
}

type itemSeller struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type itemCondition struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type itemShipPayer struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type itemCategory struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type itemTranslation struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}
