package bot

import (
	"fmt"

	"github.com/ilyasaftr/mercari-price-tracking/domain"
	"github.com/ilyasaftr/mercari-price-tracking/internal/textutil"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func kb(rows ...[]tgbotapi.KeyboardButton) tgbotapi.ReplyKeyboardMarkup {
	m := tgbotapi.NewReplyKeyboard(rows...)
	m.ResizeKeyboard = true
	return m
}

func kbtn(text string) tgbotapi.KeyboardButton {
	return tgbotapi.NewKeyboardButton(text)
}

// --- Reply Keyboards ---

func mainMenu() tgbotapi.ReplyKeyboardMarkup {
	return kb(
		tgbotapi.NewKeyboardButtonRow(kbtn("➕ Track"), kbtn("📋 My Tracks")),
		tgbotapi.NewKeyboardButtonRow(kbtn("🔄 Check Now"), kbtn("🔍 Search")),
	)
}

type sortOption struct {
	Label string
	Sort  string
	Order string
}

var sortOptions = []sortOption{
	{"🕐 Newest first", "SORT_CREATED_TIME", "ORDER_DESC"},
	{"🕐 Oldest first", "SORT_CREATED_TIME", "ORDER_ASC"},
	{"💰 Cheapest first", "SORT_PRICE", "ORDER_ASC"},
	{"💰 Priciest first", "SORT_PRICE", "ORDER_DESC"},
	{"⭐ Best match", "SORT_SCORE", "ORDER_DESC"},
	{"❤️ Most liked", "SORT_NUM_LIKES", "ORDER_DESC"},
}

func sortKeyboard() tgbotapi.ReplyKeyboardMarkup {
	return kb(
		tgbotapi.NewKeyboardButtonRow(kbtn(sortOptions[0].Label), kbtn(sortOptions[1].Label)),
		tgbotapi.NewKeyboardButtonRow(kbtn(sortOptions[2].Label), kbtn(sortOptions[3].Label)),
		tgbotapi.NewKeyboardButtonRow(kbtn(sortOptions[4].Label), kbtn(sortOptions[5].Label)),
		tgbotapi.NewKeyboardButtonRow(kbtn("⬅️ Back"), kbtn("❌ Cancel")),
	)
}

func findSortOption(label string) *sortOption {
	for i := range sortOptions {
		if sortOptions[i].Label == label {
			return &sortOptions[i]
		}
	}
	return nil
}

type conditionOption struct {
	Label string
	Desc  string
	IDs   []int
}

var conditionOptions = []conditionOption{
	{"Any condition", "No filter", nil},
	{"New / Unused", "Brand new, never used", []int{1}},
	{"Like New", "Used once or twice", []int{2}},
	{"Good & above", "New + Like New + Good", []int{1, 2, 3}},
}

func conditionKeyboard() tgbotapi.ReplyKeyboardMarkup {
	return kb(
		tgbotapi.NewKeyboardButtonRow(kbtn(conditionOptions[0].Label), kbtn(conditionOptions[1].Label)),
		tgbotapi.NewKeyboardButtonRow(kbtn(conditionOptions[2].Label), kbtn(conditionOptions[3].Label)),
		tgbotapi.NewKeyboardButtonRow(kbtn("⬅️ Back"), kbtn("❌ Cancel")),
	)
}

func findConditionOption(label string) *conditionOption {
	for i := range conditionOptions {
		if conditionOptions[i].Label == label {
			return &conditionOptions[i]
		}
	}
	return nil
}

func conditionLabel(ids []int) string {
	for _, o := range conditionOptions {
		if intsEqual(ids, o.IDs) {
			return o.Label
		}
	}
	if len(ids) == 0 {
		return "Any condition"
	}
	return fmt.Sprintf("%d selected", len(ids))
}

func optionsKeyboard() tgbotapi.ReplyKeyboardMarkup {
	return kb(
		tgbotapi.NewKeyboardButtonRow(kbtn("💰 Set Price Range"), kbtn("🚫 Exclude Words")),
		tgbotapi.NewKeyboardButtonRow(kbtn("✅ Start Tracking"), kbtn("❌ Cancel")),
	)
}

func trackListKeyboard(tracks []domain.TrackedSearch) tgbotapi.ReplyKeyboardMarkup {
	var rows [][]tgbotapi.KeyboardButton
	for i := 0; i < len(tracks); i += 2 {
		params := domain.ParseInput(tracks[i].Keyword)
		label := fmt.Sprintf("#%d %s", tracks[i].ID, textutil.Truncate(params.DisplayName(), 25))
		if i+1 < len(tracks) {
			params2 := domain.ParseInput(tracks[i+1].Keyword)
			label2 := fmt.Sprintf("#%d %s", tracks[i+1].ID, textutil.Truncate(params2.DisplayName(), 25))
			rows = append(rows, tgbotapi.NewKeyboardButtonRow(kbtn(label), kbtn(label2)))
		} else {
			rows = append(rows, tgbotapi.NewKeyboardButtonRow(kbtn(label)))
		}
	}
	rows = append(rows, tgbotapi.NewKeyboardButtonRow(kbtn("🔄 Check Now"), kbtn("⬅️ Back")))
	return kb(rows...)
}

func trackActionsKeyboard(hasNotifyKeywords bool) tgbotapi.ReplyKeyboardMarkup {
	row1 := tgbotapi.NewKeyboardButtonRow(kbtn("🔔 Notify Keyword"))
	if hasNotifyKeywords {
		row1 = tgbotapi.NewKeyboardButtonRow(kbtn("🔔 Notify Keyword"), kbtn("📋 View Keywords"))
	}
	return kb(
		row1,
		tgbotapi.NewKeyboardButtonRow(kbtn("❌ Untrack"), kbtn("⬅️ Back")),
	)
}

func matchTypeKeyboard() tgbotapi.ReplyKeyboardMarkup {
	return kb(
		tgbotapi.NewKeyboardButtonRow(kbtn("📝 Title only")),
		tgbotapi.NewKeyboardButtonRow(kbtn("📄 Description only")),
		tgbotapi.NewKeyboardButtonRow(kbtn("📝📄 Both (title + desc)")),
		tgbotapi.NewKeyboardButtonRow(kbtn("❌ Cancel")),
	)
}

var matchTypeMap = map[string]domain.MatchField{
	"📝 Title only":           domain.MatchTitle,
	"📄 Description only":     domain.MatchDesc,
	"📝📄 Both (title + desc)": domain.MatchBoth,
}

var matchFieldLabel = map[domain.MatchField]string{
	domain.MatchTitle: "📝 title",
	domain.MatchDesc:  "📄 desc",
	domain.MatchBoth:  "📝📄 both",
}

func alertListKeyboard(alerts []domain.AlertKeyword) tgbotapi.ReplyKeyboardMarkup {
	var rows [][]tgbotapi.KeyboardButton
	for _, a := range alerts {
		label := fmt.Sprintf("❌ #%d %s [%s]", a.ID, textutil.Truncate(a.Keyword, 20), matchFieldLabel[a.MatchField])
		rows = append(rows, tgbotapi.NewKeyboardButtonRow(kbtn(label)))
	}
	rows = append(rows, tgbotapi.NewKeyboardButtonRow(kbtn("⬅️ Back")))
	return kb(rows...)
}

func intsEqual(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
