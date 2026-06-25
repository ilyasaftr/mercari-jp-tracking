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

func (h *Handler) showTrackList(ctx context.Context, chatID int64) {
	searches, err := h.svc.ListTracked(ctx, chatID)
	if err != nil {
		h.sendMsg(chatID, fmt.Sprintf("❌ Error: %v", err), mainMenu())
		return
	}

	if len(searches) == 0 {
		h.state.clear(chatID)
		h.sendMsg(chatID, "📋 No tracked searches yet.\nUse ➕ Track to add one.", mainMenu())
		return
	}

	allAlerts, _ := h.svc.ListAlerts(ctx, chatID)
	alertsBySearch := make(map[int64][]domain.AlertKeyword)
	for _, a := range allAlerts {
		alertsBySearch[a.SearchID] = append(alertsBySearch[a.SearchID], a)
	}

	var sb strings.Builder
	sb.WriteString("📋 *My Tracks*\n\nTap a track to manage it:\n\n")

	for _, s := range searches {
		params := domain.ParseInput(s.Keyword)
		sb.WriteString(fmt.Sprintf("*#%d* %s", s.ID, textutil.EscapeMarkdown(params.DisplayName())))

		searchAlerts := alertsBySearch[s.ID]
		if len(searchAlerts) > 0 {
			for _, a := range searchAlerts {
				sb.WriteString(fmt.Sprintf("\n   🔔 `%s` [%s]", textutil.EscapeMarkdown(a.Keyword), matchFieldLabel[a.MatchField]))
			}
		}
		sb.WriteString("\n\n")
	}

	h.state.set(chatID, &chatState{View: viewTrackList, Tracks: searches})
	h.sendMsg(chatID, sb.String(), trackListKeyboard(searches))
}

func (h *Handler) handleTrackListPick(ctx context.Context, chatID int64, text string) {
	if text == "🔄 Check Now" {
		h.state.clear(chatID)
		if err := h.svc.CheckAll(ctx); err != nil {
			h.sendMsg(chatID, fmt.Sprintf("❌ Error: %v", err), mainMenu())
		} else {
			h.sendMsg(chatID, "✅ Check completed", mainMenu())
		}
		return
	}

	if !strings.HasPrefix(text, "#") {
		h.goMain(chatID)
		return
	}

	idStr := strings.SplitN(text[1:], " ", 2)[0]
	searchID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		return
	}

	h.showTrackActions(ctx, chatID, searchID)
}

func (h *Handler) showTrackActions(ctx context.Context, chatID int64, searchID int64) {
	ts, err := h.svc.GetTrackedSearch(ctx, searchID)
	if err != nil || ts == nil || ts.ChatID != chatID {
		h.sendMsg(chatID, "❌ Track not found.", mainMenu())
		h.state.clear(chatID)
		return
	}

	alerts, _ := h.svc.ListAlertsBySearch(ctx, searchID)
	params := domain.ParseInput(ts.Keyword)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("📌 *Track #%d*\n\n", searchID))
	sb.WriteString(fmt.Sprintf("📝 %s\n", textutil.EscapeMarkdown(params.DisplayName())))

	if len(alerts) > 0 {
		sb.WriteString("\n🔔 *Notify Keywords:*\n")
		for _, a := range alerts {
			sb.WriteString(fmt.Sprintf("  • `%s` [%s]\n", textutil.EscapeMarkdown(a.Keyword), matchFieldLabel[a.MatchField]))
		}
	} else {
		sb.WriteString("\n_No notify keywords — all items notified._")
	}

	h.state.set(chatID, &chatState{View: viewTrackActions, SearchID: searchID})
	h.sendMsg(chatID, sb.String(), trackActionsKeyboard(len(alerts) > 0))
}

func (h *Handler) handleTrackActionPick(ctx context.Context, chatID int64, text string) {
	st := h.state.get(chatID)
	if st == nil {
		h.goMain(chatID)
		return
	}

	switch text {
	case "🔔 Notify Keyword":
		st.View = viewWaitAlertKeyword
		h.sendMsg(chatID, "🔔 Send keywords — you'll only be notified when items match.\n\nSeparate multiple with commas:\n`plush, keychain, charm`", tgbotapi.NewRemoveKeyboard(true))

	case "📋 View Keywords":
		h.showAlertList(ctx, chatID, st.SearchID)

	case "❌ Untrack":
		if err := h.svc.Untrack(ctx, st.SearchID, chatID); err != nil {
			h.sendMsg(chatID, fmt.Sprintf("❌ Error: %v", err), mainMenu())
		} else {
			h.sendMsg(chatID, "✅ Untracked.", mainMenu())
		}
		h.showTrackList(ctx, chatID)
	}
}

func (h *Handler) handleAlertKeywordInput(chatID int64, text string) {
	st := h.state.get(chatID)
	if st == nil {
		h.goMain(chatID)
		return
	}

	st.PendingAlert = text
	st.View = viewAlertMatchType

	keywords := splitKeywords(text)
	preview := "`" + textutil.EscapeMarkdown(strings.Join(keywords, "`, `")) + "`"
	h.sendMsg(chatID,
		fmt.Sprintf("🔔 Keywords: %s\n\nWhere should I look for these?", preview),
		matchTypeKeyboard())
}

func (h *Handler) handleMatchTypePick(ctx context.Context, chatID int64, text string) {
	st := h.state.get(chatID)
	if st == nil || st.PendingAlert == "" {
		h.goMain(chatID)
		return
	}

	matchField, ok := matchTypeMap[text]
	if !ok {
		h.sendMsg(chatID, "Please tap one of the options below.", matchTypeKeyboard())
		return
	}

	keywords := splitKeywords(st.PendingAlert)
	searchID := st.SearchID
	var added []string
	var newAlerts []domain.AlertKeyword

	for _, kw := range keywords {
		if err := h.svc.AddAlert(ctx, searchID, kw, matchField, chatID); err != nil {
			continue
		}
		added = append(added, kw)
		newAlerts = append(newAlerts, domain.AlertKeyword{
			SearchID:   searchID,
			ChatID:     chatID,
			Keyword:    kw,
			MatchField: matchField,
		})
	}

	h.state.clear(chatID)

	if len(added) == 0 {
		h.sendMsg(chatID, "❌ No keywords added (may already exist).", mainMenu())
	} else {
		label := matchFieldLabel[matchField]
		preview := "`" + textutil.EscapeMarkdown(strings.Join(added, "`, `")) + "`"
		h.sendMsg(chatID, fmt.Sprintf("✅ Added %d keyword(s): %s [%s]\nScanning existing items...", len(added), preview, label), mainMenu())

		if err := h.svc.ScanAlertKeywords(ctx, searchID, chatID, newAlerts); err != nil {
			log.Printf("scan alert keywords: %v", err)
		}
	}
	h.showTrackActions(ctx, chatID, searchID)
}

func (h *Handler) showAlertList(ctx context.Context, chatID int64, searchID int64) {
	alerts, err := h.svc.ListAlertsBySearch(ctx, searchID)
	if err != nil {
		h.sendMsg(chatID, fmt.Sprintf("❌ Error: %v", err), mainMenu())
		return
	}

	if len(alerts) == 0 {
		h.sendMsg(chatID, "No notify keywords on this track.", mainMenu())
		return
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("🔔 *Notify Keywords for track #%d*\n\n", searchID))
	for _, a := range alerts {
		sb.WriteString(fmt.Sprintf("• #%d `%s` — %s\n", a.ID, textutil.EscapeMarkdown(a.Keyword), matchFieldLabel[a.MatchField]))
	}
	sb.WriteString("\n_Tap to remove:_")

	h.state.set(chatID, &chatState{View: viewAlertList, SearchID: searchID, Alerts: alerts})
	h.sendMsg(chatID, sb.String(), alertListKeyboard(alerts))
}

func (h *Handler) handleAlertListPick(ctx context.Context, chatID int64, text string) {
	if !strings.HasPrefix(text, "❌ #") {
		return
	}

	idStr := strings.SplitN(text[len("❌ #"):], " ", 2)[0]
	alertID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		return
	}

	st := h.state.get(chatID)
	searchID := int64(0)
	if st != nil {
		searchID = st.SearchID
	}

	if err := h.svc.RemoveAlert(ctx, alertID, chatID); err != nil {
		h.sendMsg(chatID, fmt.Sprintf("❌ Error: %v", err), mainMenu())
		return
	}

	h.sendMsg(chatID, "✅ Keyword removed.", mainMenu())
	if searchID > 0 {
		h.showAlertList(ctx, chatID, searchID)
	}
}

func (h *Handler) doSearch(ctx context.Context, chatID int64, input string) {
	items, err := h.svc.SearchNow(ctx, input)
	if err != nil {
		h.sendMsg(chatID, fmt.Sprintf("❌ Error: %v", err), mainMenu())
		return
	}
	if len(items) == 0 {
		h.sendMsg(chatID, "No items found.", mainMenu())
		return
	}

	params := domain.ParseInput(input)
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("🔍 *%d result(s)* for `%s`\n\n", len(items), textutil.EscapeMarkdown(params.DisplayName())))
	for i, item := range items {
		if i >= 10 {
			sb.WriteString(fmt.Sprintf("\n_...and %d more_", len(items)-10))
			break
		}
		sb.WriteString(fmt.Sprintf(
			"• [%s](%s)\n  💰 ¥%d\n\n",
			textutil.EscapeMarkdown(textutil.Truncate(item.Name, 50)),
			item.ItemURL,
			item.Price,
		))
	}

	h.sendMsg(chatID, sb.String(), mainMenu())
}
