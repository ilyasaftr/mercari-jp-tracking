package bot

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/ilyasaftr/mercari-price-tracking/domain"
	"github.com/ilyasaftr/mercari-price-tracking/internal/textutil"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func (h *Handler) startTrackSetup(chatID int64, input string) {
	params := domain.ParseInput(input)

	if params.ItemID != "" {
		h.state.clear(chatID)
		h.sendMsg(chatID, fmt.Sprintf("✅ Tracking item: `%s`", params.ItemID), mainMenu())
		return
	}

	if params.Sort == "" {
		params.Sort = "SORT_CREATED_TIME"
	}
	if params.Order == "" {
		params.Order = "ORDER_DESC"
	}
	if len(params.Status) == 0 {
		params.Status = []string{"STATUS_ON_SALE"}
	}

	h.state.set(chatID, &chatState{
		View:  viewTrackSort,
		Setup: &trackSetup{Input: input, Params: params},
	})

	h.sendMsg(chatID, fmt.Sprintf("⚙️ *Track Setup*\n\n📝 Keyword: `%s`\n\n*Step 1/3* — Pick a sort order:", textutil.EscapeMarkdown(params.DisplayName())), sortKeyboard())
}

func (h *Handler) handleSortPick(chatID int64, text string) {
	st := h.state.get(chatID)
	if st == nil || st.Setup == nil {
		h.goMain(chatID)
		return
	}

	opt := findSortOption(text)
	if opt == nil {
		h.sendMsg(chatID, "Please tap one of the sort buttons below.", sortKeyboard())
		return
	}

	st.Setup.Params.Sort = opt.Sort
	st.Setup.Params.Order = opt.Order
	st.View = viewTrackCondition

	h.sendMsg(chatID, fmt.Sprintf(
		"✅ Sort: %s\n\n*Step 2/3* — Item condition:\n\n"+
			"• *Any condition* — No filter\n"+
			"• *New / Unused* — Brand new, never used\n"+
			"• *Like New* — Used once or twice\n"+
			"• *Good & above* — New + Like New + Good",
		opt.Label,
	), conditionKeyboard())
}

func (h *Handler) handleConditionPick(chatID int64, text string) {
	st := h.state.get(chatID)
	if st == nil || st.Setup == nil {
		h.goMain(chatID)
		return
	}

	opt := findConditionOption(text)
	if opt == nil {
		h.sendMsg(chatID, "Please tap one of the condition buttons below.", conditionKeyboard())
		return
	}

	st.Setup.Params.ItemConditionID = opt.IDs
	st.View = viewTrackOptions

	h.sendMsg(chatID, h.buildSummary(st.Setup.Params), optionsKeyboard())
}

func (h *Handler) handleOptionsPick(ctx context.Context, chatID int64, text string) {
	st := h.state.get(chatID)
	if st == nil || st.Setup == nil {
		h.goMain(chatID)
		return
	}

	switch text {
	case "✅ Start Tracking":
		h.confirmTrack(ctx, chatID)
	case "💰 Set Price Range":
		st.View = viewWaitPriceRange
		h.sendMsg(chatID, "💰 Send price range:\n\n"+
			"• `1000-5000` — min to max\n"+
			"• `1000` — min only\n"+
			"• `0-5000` — max only\n"+
			"• `0` — clear price filter", tgbotapi.NewRemoveKeyboard(true))
	case "🚫 Exclude Words":
		st.View = viewWaitExcludeKeyword
		h.sendMsg(chatID, "🚫 Send keywords to exclude from results:\n\n"+
			"• Example: `plush doll`\n"+
			"• Send `0` to clear", tgbotapi.NewRemoveKeyboard(true))
	default:
		h.sendMsg(chatID, "Tap a button below.", optionsKeyboard())
	}
}

func (h *Handler) handlePriceInput(chatID int64, text string) {
	st := h.state.get(chatID)
	if st == nil || st.Setup == nil {
		h.goMain(chatID)
		return
	}

	parts := strings.SplitN(text, "-", 2)
	min, _ := strconv.Atoi(strings.TrimSpace(parts[0]))
	max := 0
	if len(parts) == 2 {
		max, _ = strconv.Atoi(strings.TrimSpace(parts[1]))
	}

	st.Setup.Params.PriceMin = min
	st.Setup.Params.PriceMax = max
	st.View = viewTrackOptions

	h.sendMsg(chatID, h.buildSummary(st.Setup.Params), optionsKeyboard())
}

func (h *Handler) handleExcludeInput(chatID int64, text string) {
	st := h.state.get(chatID)
	if st == nil || st.Setup == nil {
		h.goMain(chatID)
		return
	}

	if text == "0" {
		st.Setup.Params.ExcludeKeyword = ""
	} else {
		st.Setup.Params.ExcludeKeyword = text
	}
	st.View = viewTrackOptions

	h.sendMsg(chatID, h.buildSummary(st.Setup.Params), optionsKeyboard())
}

func (h *Handler) buildSummary(p domain.SearchParams) string {
	var sb strings.Builder
	sb.WriteString("⚙️ *Track Summary*\n\n")

	if p.Keyword != "" {
		sb.WriteString(fmt.Sprintf("📝 Keyword: `%s`\n", textutil.EscapeMarkdown(p.Keyword)))
	}

	for _, o := range sortOptions {
		if p.Sort == o.Sort && p.Order == o.Order {
			sb.WriteString(fmt.Sprintf("📊 Sort: %s\n", o.Label))
			break
		}
	}

	sb.WriteString(fmt.Sprintf("📦 Condition: %s\n", conditionLabel(p.ItemConditionID)))

	if p.PriceMin > 0 || p.PriceMax > 0 {
		sb.WriteString("💰 Price: ")
		if p.PriceMin > 0 {
			sb.WriteString(fmt.Sprintf("¥%d", p.PriceMin))
		} else {
			sb.WriteString("any")
		}
		sb.WriteString(" — ")
		if p.PriceMax > 0 {
			sb.WriteString(fmt.Sprintf("¥%d", p.PriceMax))
		} else {
			sb.WriteString("any")
		}
		sb.WriteString("\n")
	}

	if p.ExcludeKeyword != "" {
		sb.WriteString(fmt.Sprintf("🚫 Exclude: `%s`\n", textutil.EscapeMarkdown(p.ExcludeKeyword)))
	}

	sb.WriteString("\n_Tap ✅ to start tracking, or set more options._")
	return sb.String()
}

func (h *Handler) confirmTrack(ctx context.Context, chatID int64) {
	st := h.state.get(chatID)
	if st == nil || st.Setup == nil {
		h.goMain(chatID)
		return
	}

	setup := st.Setup
	finalInput := domain.BuildSearchURL(setup.Params)

	searchID, err := h.svc.Track(ctx, finalInput, chatID)
	if err != nil {
		h.sendMsg(chatID, fmt.Sprintf("❌ Error: %v", err), mainMenu())
		h.state.clear(chatID)
		return
	}

	h.state.clear(chatID)
	displayName := textutil.EscapeMarkdown(setup.Params.DisplayName())

	if searchID > 0 {
		h.showTrackActions(ctx, chatID, searchID)
		h.sendMsg(chatID, fmt.Sprintf("✅ *Now tracking!*\n📝 `%s`\n⏳ Scanning existing items...", displayName), nil)
	} else {
		h.sendMsg(chatID, fmt.Sprintf("✅ *Now tracking!*\n📝 `%s`", displayName), mainMenu())
	}

	go func() {
		ts := domain.TrackedSearch{ID: searchID, Keyword: finalInput, ChatID: chatID}
		itemCount, err := h.svc.SeedSearch(ctx, ts)
		if err != nil {
			log.Printf("seed search failed: %v", err)
			return
		}
		h.sendMsg(chatID, fmt.Sprintf("📦 Found *%d* existing items for `%s`\n\nYou'll be notified when NEW items appear or prices change.", itemCount, displayName), nil)
	}()
}
