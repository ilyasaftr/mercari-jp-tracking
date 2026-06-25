package bot

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/ilyasaftr/mercari-price-tracking/domain"
	"github.com/ilyasaftr/mercari-price-tracking/internal/textutil"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type TrackerService interface {
	Track(ctx context.Context, input string, chatID int64) (int64, error)
	Untrack(ctx context.Context, id int64, chatID int64) error
	SeedSearch(ctx context.Context, ts domain.TrackedSearch) (int, error)
	ListTracked(ctx context.Context, chatID int64) ([]domain.TrackedSearch, error)
	GetTrackedSearch(ctx context.Context, id int64) (*domain.TrackedSearch, error)
	CheckAll(ctx context.Context) error
	SearchNow(ctx context.Context, input string) ([]domain.Item, error)
	AddAlert(ctx context.Context, searchID int64, keyword string, matchField domain.MatchField, chatID int64) error
	RemoveAlert(ctx context.Context, id int64, chatID int64) error
	ListAlerts(ctx context.Context, chatID int64) ([]domain.AlertKeyword, error)
	ListAlertsBySearch(ctx context.Context, searchID int64) ([]domain.AlertKeyword, error)
	ScanAlertKeywords(ctx context.Context, searchID int64, chatID int64, keywords []domain.AlertKeyword) error
}

type Handler struct {
	bot   *tgbotapi.BotAPI
	svc   TrackerService
	state *stateManager
}

func NewHandler(bot *tgbotapi.BotAPI, svc TrackerService) *Handler {
	h := &Handler{
		bot:   bot,
		svc:   svc,
		state: newStateManager(),
	}
	h.registerCommands()
	return h
}

func (h *Handler) registerCommands() {
	cmds := tgbotapi.NewSetMyCommands(
		tgbotapi.BotCommand{Command: "start", Description: "Show main menu"},
		tgbotapi.BotCommand{Command: "track", Description: "Track a keyword or URL"},
		tgbotapi.BotCommand{Command: "list", Description: "Show tracked searches"},
		tgbotapi.BotCommand{Command: "check", Description: "Run price check now"},
	)
	if _, err := h.bot.Request(cmds); err != nil {
		log.Printf("set commands: %v", err)
	}
}

func (h *Handler) Run(ctx context.Context) {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates := h.bot.GetUpdatesChan(u)

	for {
		select {
		case <-ctx.Done():
			return
		case update := <-updates:
			if update.Message != nil {
				if update.Message.IsCommand() {
					go h.handleCommand(ctx, update.Message)
				} else {
					go h.handleText(ctx, update.Message)
				}
			}
		}
	}
}

func (h *Handler) handleCommand(ctx context.Context, msg *tgbotapi.Message) {
	chatID := msg.Chat.ID
	args := msg.CommandArguments()

	switch msg.Command() {
	case "start":
		h.goMain(chatID)
	case "track":
		if args != "" {
			h.startTrackSetup(chatID, args)
		} else {
			h.state.set(chatID, &chatState{View: viewWaitTrackKeyword})
			h.sendMsg(chatID, "🔍 Send me a *keyword* or *Mercari URL* to track:", tgbotapi.NewRemoveKeyboard(true))
		}
	case "list":
		h.showTrackList(ctx, chatID)
	case "check":
		if err := h.svc.CheckAll(ctx); err != nil {
			h.sendMsg(chatID, fmt.Sprintf("❌ Error: %v", err), mainMenu())
		} else {
			h.sendMsg(chatID, "✅ Price check completed", mainMenu())
		}
	default:
		h.sendMsg(chatID, "Unknown command. Tap a button or use /start.", mainMenu())
	}
}

func (h *Handler) handleText(ctx context.Context, msg *tgbotapi.Message) {
	chatID := msg.Chat.ID
	text := strings.TrimSpace(msg.Text)

	switch text {
	case "⬅️ Back":
		h.goBack(ctx, chatID)
		return
	case "❌ Cancel":
		h.state.clear(chatID)
		h.sendMsg(chatID, "❌ Cancelled.", mainMenu())
		return
	}

	st := h.state.get(chatID)
	view := viewMain
	if st != nil {
		view = st.View
	}

	switch view {
	case viewMain:
		h.handleMainView(ctx, chatID, text)
	case viewWaitTrackKeyword:
		h.startTrackSetup(chatID, text)
	case viewTrackSort:
		h.handleSortPick(chatID, text)
	case viewTrackCondition:
		h.handleConditionPick(chatID, text)
	case viewTrackOptions:
		h.handleOptionsPick(ctx, chatID, text)
	case viewWaitPriceRange:
		h.handlePriceInput(chatID, text)
	case viewWaitExcludeKeyword:
		h.handleExcludeInput(chatID, text)
	case viewTrackList:
		h.handleTrackListPick(ctx, chatID, text)
	case viewTrackActions:
		h.handleTrackActionPick(ctx, chatID, text)
	case viewWaitAlertKeyword:
		h.handleAlertKeywordInput(chatID, text)
	case viewAlertMatchType:
		h.handleMatchTypePick(ctx, chatID, text)
	case viewAlertList:
		h.handleAlertListPick(ctx, chatID, text)
	default:
		h.goMain(chatID)
	}
}

func (h *Handler) goMain(chatID int64) {
	h.state.clear(chatID)
	h.sendMsg(chatID, "🔍 *Mercari Price Tracker*\n\n"+
		"Track Mercari searches and get notified on new items & price changes.\n\n"+
		"Tap a button below to get started.", mainMenu())
}

func (h *Handler) goBack(ctx context.Context, chatID int64) {
	st := h.state.get(chatID)
	if st == nil {
		h.goMain(chatID)
		return
	}
	switch st.View {
	case viewTrackList, viewWaitTrackKeyword:
		h.goMain(chatID)
	case viewTrackSort:
		h.state.set(chatID, &chatState{View: viewWaitTrackKeyword})
		h.sendMsg(chatID, "🔍 Send me a *keyword* or *Mercari URL* to track:", tgbotapi.NewRemoveKeyboard(true))
	case viewTrackCondition:
		if st.Setup != nil {
			st.View = viewTrackSort
			h.sendMsg(chatID, fmt.Sprintf("⚙️ *Track Setup*\n\n📝 Keyword: `%s`\n\n*Step 1/3* — Pick a sort order:", textutil.EscapeMarkdown(st.Setup.Params.DisplayName())), sortKeyboard())
		} else {
			h.goMain(chatID)
		}
	case viewTrackOptions, viewWaitPriceRange, viewWaitExcludeKeyword:
		if st.Setup != nil {
			st.View = viewTrackCondition
			h.sendMsg(chatID, "*Step 2/3* — Item condition:\n\n"+
				"• *Any condition* — No filter\n"+
				"• *New / Unused* — Brand new, never used\n"+
				"• *Like New* — Used once or twice\n"+
				"• *Good & above* — New + Like New + Good", conditionKeyboard())
		} else {
			h.goMain(chatID)
		}
	case viewTrackActions:
		h.showTrackList(ctx, chatID)
	case viewAlertList, viewAlertMatchType, viewWaitAlertKeyword:
		h.showTrackActions(ctx, chatID, st.SearchID)
	default:
		h.goMain(chatID)
	}
}

func (h *Handler) handleMainView(ctx context.Context, chatID int64, text string) {
	switch text {
	case "➕ Track":
		h.state.set(chatID, &chatState{View: viewWaitTrackKeyword})
		h.sendMsg(chatID, "🔍 Send me a *keyword* or *Mercari URL* to track:", tgbotapi.NewRemoveKeyboard(true))
	case "📋 My Tracks":
		h.showTrackList(ctx, chatID)
	case "🔄 Check Now":
		if err := h.svc.CheckAll(ctx); err != nil {
			h.sendMsg(chatID, fmt.Sprintf("❌ Error: %v", err), mainMenu())
		} else {
			h.sendMsg(chatID, "✅ Price check completed", mainMenu())
		}
	case "🔍 Search":
		h.state.set(chatID, &chatState{View: viewWaitTrackKeyword})
		h.sendMsg(chatID, "🔍 Send a *keyword* or *Mercari URL* to search:", tgbotapi.NewRemoveKeyboard(true))
	default:
		if domain.IsMercariURL(text) {
			h.doSearch(ctx, chatID, text)
		}
	}
}

func (h *Handler) sendMsg(chatID int64, text string, markup any) {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = tgbotapi.ModeMarkdown
	msg.DisableWebPagePreview = true
	if markup != nil {
		msg.ReplyMarkup = markup
	}
	if _, err := h.bot.Send(msg); err != nil {
		log.Printf("send: %v", err)
	}
}

func splitKeywords(input string) []string {
	var result []string
	for _, kw := range strings.Split(input, ",") {
		kw = strings.TrimSpace(kw)
		if kw != "" {
			result = append(result, kw)
		}
	}
	return result
}
