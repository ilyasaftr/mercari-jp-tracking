package domain

import "time"

type Item struct {
	ID            string
	Name          string
	NameEN        string
	Description   string
	DescriptionEN string
	Price         int
	PriceUSD      float64
	Status        string
	ThumbnailURL  string
	PhotoURL      string
	ItemURL       string
	SellerID      string
	SellerName    string
	CategoryID    string
	ConditionID   string
	Condition     string
	ShippingPayer string
	NumLikes      int
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type PriceRecord struct {
	ItemID    string
	Price     int
	RecordedAt time.Time
}

type TrackedSearch struct {
	ID        int64
	Keyword   string
	ChatID    int64
	CreatedAt time.Time
}

type PriceChange struct {
	Item     Item
	OldPrice int
	NewPrice int
}

type MatchField string

const (
	MatchTitle MatchField = "title"
	MatchDesc  MatchField = "desc"
	MatchBoth  MatchField = "both"
)

type AlertKeyword struct {
	ID         int64
	SearchID   int64
	ChatID     int64
	Keyword    string
	MatchField MatchField
	CreatedAt  time.Time
}
