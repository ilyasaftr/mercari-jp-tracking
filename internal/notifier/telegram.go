package notifier

import (
	"context"
	"fmt"
	"strings"

	"github.com/ilyasaftr/mercari-price-tracking/domain"
	"github.com/ilyasaftr/mercari-price-tracking/internal/textutil"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type Telegram struct {
	bot *tgbotapi.BotAPI
}

func NewTelegram(bot *tgbotapi.BotAPI) *Telegram {
	return &Telegram{bot: bot}
}

func (t *Telegram) NotifyNewItems(_ context.Context, chatID int64, keyword string, items []domain.Item) error {
	if len(items) == 0 {
		return nil
	}

	header := fmt.Sprintf("🆕 *%d new item(s)* for `%s`", len(items), textutil.EscapeMarkdown(keyword))
	if err := t.send(chatID, header); err != nil {
		return err
	}

	for i, item := range items {
		if i >= 10 {
			t.send(chatID, fmt.Sprintf("_...and %d more_", len(items)-10))
			break
		}
		t.sendItemCard(chatID, item)
	}
	return nil
}

func (t *Telegram) NotifyPriceChanges(_ context.Context, chatID int64, keyword string, changes []domain.PriceChange) error {
	if len(changes) == 0 {
		return nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("💰 *%d price change(s)* for `%s`\n\n", len(changes), textutil.EscapeMarkdown(keyword)))

	for i, ch := range changes {
		if i >= 10 {
			sb.WriteString(fmt.Sprintf("\n_...and %d more_", len(changes)-10))
			break
		}

		arrow := "📉"
		if ch.NewPrice > ch.OldPrice {
			arrow = "📈"
		}

		name := ch.Item.Name
		if ch.Item.NameEN != "" {
			name = ch.Item.NameEN
		}
		priceStr := fmt.Sprintf("¥%d → ¥%d", ch.OldPrice, ch.NewPrice)
		if ch.Item.PriceUSD > 0 {
			oldUSD := float64(ch.OldPrice) / float64(ch.NewPrice) * ch.Item.PriceUSD
			priceStr = fmt.Sprintf("¥%d (~$%.2f) → ¥%d (~$%.2f)", ch.OldPrice, oldUSD, ch.NewPrice, ch.Item.PriceUSD)
		}
		sb.WriteString(fmt.Sprintf(
			"• [%s](%s)\n  %s %s\n\n",
			textutil.EscapeMarkdown(textutil.Truncate(name, 60)),
			ch.Item.ItemURL,
			arrow,
			priceStr,
		))
	}

	return t.send(chatID, sb.String())
}

func (t *Telegram) sendItemCard(chatID int64, item domain.Item) {
	var sb strings.Builder

	if item.NameEN != "" {
		sb.WriteString(fmt.Sprintf("*%s*\n", textutil.EscapeMarkdown(item.NameEN)))
		sb.WriteString(fmt.Sprintf("🇯🇵 %s\n", textutil.EscapeMarkdown(item.Name)))
	} else {
		sb.WriteString(fmt.Sprintf("*%s*\n", textutil.EscapeMarkdown(item.Name)))
	}
	if item.PriceUSD > 0 {
		sb.WriteString(fmt.Sprintf("💰 *¥%d* (~$%.2f)", item.Price, item.PriceUSD))
	} else {
		sb.WriteString(fmt.Sprintf("💰 *¥%d*", item.Price))
	}

	if item.Condition != "" {
		sb.WriteString(fmt.Sprintf(" • %s", textutil.EscapeMarkdown(item.Condition)))
	}
	if item.SellerName != "" {
		sb.WriteString(fmt.Sprintf(" • by %s", textutil.EscapeMarkdown(item.SellerName)))
	}
	sb.WriteString("\n")

	if item.DescriptionEN != "" {
		sb.WriteString(fmt.Sprintf("\n_%s_\n", textutil.EscapeMarkdown(strings.TrimSpace(item.DescriptionEN))))
	} else if item.Description != "" {
		sb.WriteString(fmt.Sprintf("\n_%s_\n", textutil.EscapeMarkdown(strings.TrimSpace(item.Description))))
	}

	sb.WriteString(fmt.Sprintf("\n🔗 [View on Mercari](%s)", item.ItemURL))

	text := sb.String()

	photoURL := item.PhotoURL
	if photoURL == "" {
		photoURL = item.ThumbnailURL
	}

	if photoURL != "" {
		photo := tgbotapi.NewPhoto(chatID, tgbotapi.FileURL(photoURL))
		if len(text) <= 1024 {
			photo.Caption = text
			photo.ParseMode = tgbotapi.ModeMarkdown
			if _, err := t.bot.Send(photo); err == nil {
				return
			}
		} else {
			if _, err := t.bot.Send(photo); err == nil {
				t.send(chatID, text)
				return
			}
		}
	}

	t.send(chatID, text)
}

func (t *Telegram) send(chatID int64, text string) error {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = tgbotapi.ModeMarkdown
	msg.DisableWebPagePreview = true
	_, err := t.bot.Send(msg)
	return err
}

