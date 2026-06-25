package domain

import "context"

type Searcher interface {
	Search(ctx context.Context, params SearchParams) ([]Item, error)
	GetItem(ctx context.Context, itemID string) (*Item, error)
}

type Repository interface {
	SaveItem(ctx context.Context, item Item) error
	GetItem(ctx context.Context, id string) (*Item, error)
	GetItemsBySearchID(ctx context.Context, searchID int64) ([]Item, error)
	SavePriceRecord(ctx context.Context, record PriceRecord) error
	GetPriceHistory(ctx context.Context, itemID string) ([]PriceRecord, error)
	SaveTrackedSearch(ctx context.Context, ts TrackedSearch) (int64, error)
	GetTrackedSearches(ctx context.Context) ([]TrackedSearch, error)
	GetTrackedSearchesByChatID(ctx context.Context, chatID int64) ([]TrackedSearch, error)
	GetTrackedSearch(ctx context.Context, id int64) (*TrackedSearch, error)
	DeleteTrackedSearch(ctx context.Context, id int64) error
	LinkItemToSearch(ctx context.Context, itemID string, searchID int64) error
	SaveAlertKeyword(ctx context.Context, ak AlertKeyword) error
	GetAlertsBySearchID(ctx context.Context, searchID int64) ([]AlertKeyword, error)
	GetAlertsByChatID(ctx context.Context, chatID int64) ([]AlertKeyword, error)
	DeleteAlertKeyword(ctx context.Context, id int64, chatID int64) error
}

type Notifier interface {
	NotifyNewItems(ctx context.Context, chatID int64, keyword string, items []Item) error
	NotifyPriceChanges(ctx context.Context, chatID int64, keyword string, changes []PriceChange) error
}
