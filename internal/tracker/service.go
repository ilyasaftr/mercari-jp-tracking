package tracker

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/ilyasaftr/mercari-price-tracking/domain"
)

type Service struct {
	searcher domain.Searcher
	repo     domain.Repository
	notifier domain.Notifier
}

func NewService(searcher domain.Searcher, repo domain.Repository, notifier domain.Notifier) *Service {
	return &Service{
		searcher: searcher,
		repo:     repo,
		notifier: notifier,
	}
}

func (s *Service) Track(ctx context.Context, input string, chatID int64) (int64, error) {
	return s.repo.SaveTrackedSearch(ctx, domain.TrackedSearch{
		Keyword: input,
		ChatID:  chatID,
	})
}

func (s *Service) Untrack(ctx context.Context, id int64, chatID int64) error {
	ts, err := s.repo.GetTrackedSearch(ctx, id)
	if err != nil {
		return err
	}
	if ts == nil || ts.ChatID != chatID {
		return fmt.Errorf("tracked search #%d not found", id)
	}
	return s.repo.DeleteTrackedSearch(ctx, id)
}

func (s *Service) ListTracked(ctx context.Context, chatID int64) ([]domain.TrackedSearch, error) {
	return s.repo.GetTrackedSearchesByChatID(ctx, chatID)
}

func (s *Service) GetTrackedSearch(ctx context.Context, id int64) (*domain.TrackedSearch, error) {
	return s.repo.GetTrackedSearch(ctx, id)
}

func (s *Service) SearchNow(ctx context.Context, input string) ([]domain.Item, error) {
	params := domain.ParseInput(input)
	return s.searcher.Search(ctx, params)
}

func (s *Service) GetPriceHistory(ctx context.Context, itemID string) ([]domain.PriceRecord, error) {
	return s.repo.GetPriceHistory(ctx, itemID)
}

// --- Alert Management ---

func (s *Service) AddAlert(ctx context.Context, searchID int64, keyword string, matchField domain.MatchField, chatID int64) error {
	ts, err := s.repo.GetTrackedSearch(ctx, searchID)
	if err != nil {
		return fmt.Errorf("get tracked search: %w", err)
	}
	if ts == nil || ts.ChatID != chatID {
		return fmt.Errorf("tracked search #%d not found", searchID)
	}
	if matchField == "" {
		matchField = domain.MatchBoth
	}
	return s.repo.SaveAlertKeyword(ctx, domain.AlertKeyword{
		SearchID:   searchID,
		ChatID:     chatID,
		Keyword:    keyword,
		MatchField: matchField,
	})
}

func (s *Service) RemoveAlert(ctx context.Context, id int64, chatID int64) error {
	return s.repo.DeleteAlertKeyword(ctx, id, chatID)
}

func (s *Service) ListAlerts(ctx context.Context, chatID int64) ([]domain.AlertKeyword, error) {
	return s.repo.GetAlertsByChatID(ctx, chatID)
}

func (s *Service) ListAlertsBySearch(ctx context.Context, searchID int64) ([]domain.AlertKeyword, error) {
	return s.repo.GetAlertsBySearchID(ctx, searchID)
}

// --- Seeding & Scanning ---

func (s *Service) SeedSearch(ctx context.Context, ts domain.TrackedSearch) (int, error) {
	params := domain.ParseInput(ts.Keyword)
	items, err := s.searcher.Search(ctx, params)
	if err != nil {
		return 0, fmt.Errorf("seed search %q: %w", ts.Keyword, err)
	}

	for _, item := range items {
		if err := s.persistItem(ctx, item, ts.ID); err != nil {
			return 0, err
		}
	}

	log.Printf("seeded %q: %d items", params.DisplayName(), len(items))
	return len(items), nil
}

func (s *Service) ScanAlertKeywords(ctx context.Context, searchID int64, chatID int64, keywords []domain.AlertKeyword) error {
	items, err := s.repo.GetItemsBySearchID(ctx, searchID)
	if err != nil {
		return fmt.Errorf("get items for scan: %w", err)
	}

	seen := make(map[string]bool)
	var matched []domain.Item
	for _, item := range items {
		for _, kw := range keywords {
			if !seen[item.ID] && itemMatchesAlert(item, kw) {
				matched = append(matched, item)
				seen[item.ID] = true
				break
			}
		}
	}

	if len(matched) == 0 {
		return nil
	}

	enriched := s.enrichItems(ctx, matched)

	ts, err := s.repo.GetTrackedSearch(ctx, searchID)
	if err != nil || ts == nil {
		return fmt.Errorf("get tracked search: %w", err)
	}

	params := domain.ParseInput(ts.Keyword)
	log.Printf("alert scan %q: %d/%d items matched", params.DisplayName(), len(matched), len(items))
	return s.notifier.NotifyNewItems(ctx, chatID, params.DisplayName(), enriched)
}

// --- Scheduler ---

func (s *Service) CheckAll(ctx context.Context) error {
	searches, err := s.repo.GetTrackedSearches(ctx)
	if err != nil {
		return fmt.Errorf("get tracked searches: %w", err)
	}

	for _, ts := range searches {
		if err := s.checkOne(ctx, ts); err != nil {
			log.Printf("error checking %q: %v", ts.Keyword, err)
		}
	}
	return nil
}

func (s *Service) checkOne(ctx context.Context, ts domain.TrackedSearch) error {
	params := domain.ParseInput(ts.Keyword)

	items, err := s.searcher.Search(ctx, params)
	if err != nil {
		return fmt.Errorf("search %q: %w", ts.Keyword, err)
	}

	diff, err := s.diffItems(ctx, items)
	if err != nil {
		return err
	}

	for _, item := range items {
		if err := s.persistItem(ctx, item, ts.ID); err != nil {
			return err
		}
	}

	alerts, err := s.repo.GetAlertsBySearchID(ctx, ts.ID)
	if err != nil {
		return fmt.Errorf("get alert keywords: %w", err)
	}

	displayName := params.DisplayName()
	if len(alerts) > 0 {
		s.notifyWithAlerts(ctx, ts, diff, alerts, displayName)
	} else {
		s.notifyAll(ctx, ts, diff, displayName)
	}

	return nil
}

func (s *Service) RunScheduler(ctx context.Context, interval time.Duration) {
	log.Printf("scheduler started, interval: %s", interval)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	if err := s.CheckAll(ctx); err != nil {
		log.Printf("initial check error: %v", err)
	}

	for {
		select {
		case <-ctx.Done():
			log.Println("scheduler stopped")
			return
		case <-ticker.C:
			if err := s.CheckAll(ctx); err != nil {
				log.Printf("check error: %v", err)
			}
		}
	}
}

// --- Internal Helpers ---

type diffResult struct {
	New            []domain.Item
	PriceChanges   []domain.PriceChange
	ContentUpdated []domain.Item
}

func (s *Service) diffItems(ctx context.Context, items []domain.Item) (*diffResult, error) {
	result := &diffResult{}
	for _, item := range items {
		existing, err := s.repo.GetItem(ctx, item.ID)
		if err != nil {
			return nil, fmt.Errorf("get item %s: %w", item.ID, err)
		}

		if existing == nil {
			result.New = append(result.New, item)
		} else if existing.Price != item.Price {
			result.PriceChanges = append(result.PriceChanges, domain.PriceChange{
				Item:     item,
				OldPrice: existing.Price,
				NewPrice: item.Price,
			})
		} else if !existing.UpdatedAt.Equal(item.UpdatedAt) {
			result.ContentUpdated = append(result.ContentUpdated, item)
		}
	}
	return result, nil
}

func (s *Service) persistItem(ctx context.Context, item domain.Item, searchID int64) error {
	if err := s.repo.SaveItem(ctx, item); err != nil {
		return fmt.Errorf("save item %s: %w", item.ID, err)
	}
	if err := s.repo.SavePriceRecord(ctx, domain.PriceRecord{
		ItemID:     item.ID,
		Price:      item.Price,
		RecordedAt: time.Now(),
	}); err != nil {
		return fmt.Errorf("save price %s: %w", item.ID, err)
	}
	if err := s.repo.LinkItemToSearch(ctx, item.ID, searchID); err != nil {
		return fmt.Errorf("link item %s: %w", item.ID, err)
	}
	return nil
}

func (s *Service) notifyWithAlerts(ctx context.Context, ts domain.TrackedSearch, diff *diffResult, alerts []domain.AlertKeyword, displayName string) {
	filteredNew := filterByKeywords(diff.New, alerts)
	if len(filteredNew) > 0 {
		enriched := s.enrichItems(ctx, filteredNew)
		if err := s.notifier.NotifyNewItems(ctx, ts.ChatID, displayName, enriched); err != nil {
			log.Printf("notify new items: %v", err)
		}
	}

	filteredChanges := filterChangesByKeywords(diff.PriceChanges, alerts)
	if len(filteredChanges) > 0 {
		if err := s.notifier.NotifyPriceChanges(ctx, ts.ChatID, displayName, filteredChanges); err != nil {
			log.Printf("notify price changes: %v", err)
		}
	}

	var filteredUpdated int
	if len(diff.ContentUpdated) > 0 {
		enriched := s.enrichItems(ctx, diff.ContentUpdated)
		for _, item := range enriched {
			if err := s.repo.SaveItem(ctx, item); err != nil {
				log.Printf("save updated item %s: %v", item.ID, err)
			}
		}
		matched := filterByKeywords(enriched, alerts)
		newIDSet := make(map[string]bool, len(filteredNew))
		for _, item := range filteredNew {
			newIDSet[item.ID] = true
		}
		var unique []domain.Item
		for _, item := range matched {
			if !newIDSet[item.ID] {
				unique = append(unique, item)
			}
		}
		filteredUpdated = len(unique)
		if len(unique) > 0 {
			if err := s.notifier.NotifyNewItems(ctx, ts.ChatID, displayName, unique); err != nil {
				log.Printf("notify updated items: %v", err)
			}
		}
	}

	log.Printf("checked %q: %d items, %d new (%d matched), %d price (%d matched), %d updated (%d matched)",
		displayName, len(diff.New)+len(diff.PriceChanges)+len(diff.ContentUpdated),
		len(diff.New), len(filteredNew), len(diff.PriceChanges), len(filteredChanges),
		len(diff.ContentUpdated), filteredUpdated)
}

func (s *Service) notifyAll(ctx context.Context, ts domain.TrackedSearch, diff *diffResult, displayName string) {
	if len(diff.New) > 0 {
		enriched := s.enrichItems(ctx, diff.New)
		if err := s.notifier.NotifyNewItems(ctx, ts.ChatID, displayName, enriched); err != nil {
			log.Printf("notify new items: %v", err)
		}
	}

	if len(diff.PriceChanges) > 0 {
		if err := s.notifier.NotifyPriceChanges(ctx, ts.ChatID, displayName, diff.PriceChanges); err != nil {
			log.Printf("notify price changes: %v", err)
		}
	}

	log.Printf("checked %q: %d new, %d price changes", displayName, len(diff.New), len(diff.PriceChanges))
}

func (s *Service) enrichItems(ctx context.Context, items []domain.Item) []domain.Item {
	enriched := make([]domain.Item, len(items))
	for i, item := range items {
		detail, err := s.searcher.GetItem(ctx, item.ID)
		if err != nil {
			log.Printf("enrich item %s: %v", item.ID, err)
			enriched[i] = item
			continue
		}
		if detail != nil {
			enriched[i] = *detail
		} else {
			enriched[i] = item
		}
	}
	return enriched
}

// --- Keyword Matching ---

func containsAny(kw string, fields ...string) bool {
	for _, f := range fields {
		if strings.Contains(strings.ToLower(f), kw) {
			return true
		}
	}
	return false
}

func itemMatchesAlert(item domain.Item, a domain.AlertKeyword) bool {
	kw := strings.ToLower(a.Keyword)
	switch a.MatchField {
	case domain.MatchTitle:
		return containsAny(kw, item.Name, item.NameEN)
	case domain.MatchDesc:
		return containsAny(kw, item.Description, item.DescriptionEN)
	default:
		return containsAny(kw, item.Name, item.NameEN, item.Description, item.DescriptionEN)
	}
}

func matchesAnyAlert(item domain.Item, alerts []domain.AlertKeyword) bool {
	for _, a := range alerts {
		if itemMatchesAlert(item, a) {
			return true
		}
	}
	return false
}

func filterByKeywords(items []domain.Item, alerts []domain.AlertKeyword) []domain.Item {
	var filtered []domain.Item
	for _, item := range items {
		if matchesAnyAlert(item, alerts) {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func filterChangesByKeywords(changes []domain.PriceChange, alerts []domain.AlertKeyword) []domain.PriceChange {
	var filtered []domain.PriceChange
	for _, ch := range changes {
		if matchesAnyAlert(ch.Item, alerts) {
			filtered = append(filtered, ch)
		}
	}
	return filtered
}
