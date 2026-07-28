package app

import (
	"context"
	"fmt"
	"html"
	"strconv"
	"strings"
	"time"

	"jobscout.ai/internal/core"
	tele "jobscout.ai/internal/integrations/telegram"
	"jobscout.ai/internal/store"
)

func (a *App) StartTelegramPolling(ctx context.Context) error {
	if a.tgClient == nil {
		return nil
	}
	offset := 0
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		updates, err := a.tgClient.GetUpdates(ctx, offset, 30)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			a.logger.Warn("telegram polling failed", "error", err)
			if err := sleepFor(ctx, 5*time.Second); err != nil {
				return err
			}
			continue
		}
		for _, update := range updates {
			if update.UpdateID >= offset {
				offset = update.UpdateID + 1
			}
			if err := a.handleTelegramUpdate(ctx, update); err != nil {
				a.logger.Warn("telegram update failed", "error", err)
			}
		}
	}
}

func (a *App) handleTelegramUpdate(ctx context.Context, update tele.Update) error {
	if update.Message != nil {
		return a.handleTelegramMessage(ctx, update.Message)
	}
	if update.CallbackQuery != nil {
		return a.handleTelegramCallback(ctx, update.CallbackQuery)
	}
	return nil
}

func (a *App) handleTelegramMessage(ctx context.Context, msg *tele.Message) error {
	if msg.From == nil || !a.isTelegramOwner(msg.From.ID) {
		return a.replyTelegramDenied(ctx, msg.Chat.ID)
	}
	command, arg := splitCommand(msg.Text)
	switch command {
	case "start":
		return a.replyTelegramStart(ctx, msg.Chat.ID)
	case "search":
		summary, err := a.RunSearch(ctx, arg)
		if err != nil {
			return a.tgClient.SendMessage(ctx, msg.Chat.ID, "Search failed: "+html.EscapeString(err.Error()), nil)
		}
		text := fmt.Sprintf("Search complete\nImported: %d\nDuplicates: %d\nFiltered: %d\nRecommended: %d\nErrors: %d", summary.Imported, summary.Duplicates, summary.Filtered, summary.Recommended, summary.Errors)
		if err := a.tgClient.SendMessage(ctx, msg.Chat.ID, text, nil); err != nil {
			return err
		}
		return a.sendTopRecommendedVacancy(ctx, msg.Chat.ID)
	case "recommended":
		return a.sendTopRecommendedVacancy(ctx, msg.Chat.ID)
	default:
		return a.tgClient.SendMessage(ctx, msg.Chat.ID, "Commands: /start /search /recommended", nil)
	}
}

func (a *App) handleTelegramCallback(ctx context.Context, cb *tele.CallbackQuery) error {
	if cb.From.ID != a.cfg.TelegramOwnerID {
		_ = a.tgClient.AnswerCallbackQuery(ctx, cb.ID, "Access denied")
		return ErrUnknownUser
	}
	parts := strings.Split(cb.Data, ":")
	if len(parts) != 3 || parts[0] != "vac" {
		return a.tgClient.AnswerCallbackQuery(ctx, cb.ID, "Unknown action")
	}
	vacancyID := parts[1]
	action := parts[2]
	var status core.VacancyStatus
	switch action {
	case "like":
		status = core.VacancyStatusViewed
	case "skip":
		status = core.VacancyStatusArchived
	default:
		return a.tgClient.AnswerCallbackQuery(ctx, cb.ID, "Unknown action")
	}
	item, err := a.UpdateVacancyStatus(ctx, vacancyID, status)
	if err != nil {
		_ = a.tgClient.AnswerCallbackQuery(ctx, cb.ID, html.EscapeString(err.Error()))
		return err
	}
	if cb.Message != nil {
		edited := vacancyCardText(*item)
		_ = a.tgClient.EditMessageText(ctx, cb.Message.Chat.ID, cb.Message.MessageID, edited, nil)
	}
	_ = a.tgClient.AnswerCallbackQuery(ctx, cb.ID, "Updated")
	if cb.Message != nil {
		return a.sendTopRecommendedVacancy(ctx, cb.Message.Chat.ID)
	}
	return nil
}

func (a *App) replyTelegramStart(ctx context.Context, chatID int64) error {
	profile, err := a.GetProfile(ctx)
	if err != nil {
		return a.tgClient.SendMessage(ctx, chatID, "JobScout AI is ready. Configure the profile first.", nil)
	}
	text := fmt.Sprintf("JobScout AI is ready.\nProfile: %s\nRoles: %d\nSkills: %d", html.EscapeString(profile.ID), len(profile.DesiredRoles), len(profile.PrimarySkills))
	return a.tgClient.SendMessage(ctx, chatID, text, nil)
}

func (a *App) replyTelegramDenied(ctx context.Context, chatID int64) error {
	return a.tgClient.SendMessage(ctx, chatID, "Access denied.", nil)
}

func (a *App) sendTopRecommendedVacancy(ctx context.Context, chatID int64) error {
	items, err := a.ListRecommendedVacancies(ctx, 0, 1)
	if err != nil {
		return err
	}
	if len(items) == 0 {
		return a.tgClient.SendMessage(ctx, chatID, "No recommended vacancies right now.", nil)
	}
	return a.sendVacancyCard(ctx, chatID, items[0])
}

func (a *App) sendVacancyCard(ctx context.Context, chatID int64, item store.VacancyWithMatch) error {
	return a.tgClient.SendMessage(ctx, chatID, vacancyCardText(item), vacancyCardMarkup(item))
}

func vacancyCardMarkup(item store.VacancyWithMatch) *tele.InlineKeyboardMarkup {
	urlTarget := item.Vacancy.CanonicalURL
	if strings.TrimSpace(urlTarget) == "" {
		urlTarget = item.Vacancy.SourceURL
	}
	rows := [][]tele.InlineKeyboardButton{
		{
			tele.Button("Подходит", "vac:"+item.Vacancy.ID+":like"),
			tele.Button("Пропустить", "vac:"+item.Vacancy.ID+":skip"),
		},
	}
	if strings.TrimSpace(urlTarget) != "" {
		rows = append(rows, []tele.InlineKeyboardButton{tele.URLButton("Открыть вакансию", urlTarget)})
	}
	return tele.NewInlineKeyboard(rows...)
}

func vacancyCardText(item store.VacancyWithMatch) string {
	vacancy := item.Vacancy
	company := ""
	if item.Company != nil {
		company = item.Company.DisplayName
	}
	score := "n/a"
	positive := ""
	negative := ""
	if item.Match != nil {
		score = strconv.Itoa(item.Match.TotalScore)
		positive = strings.Join(item.Match.PositiveReasons, "; ")
		negative = strings.Join(item.Match.NegativeReasons, "; ")
	}
	salary := salaryRangeLabel(vacancy.SalaryFrom, vacancy.SalaryTo, vacancy.Currency)
	link := vacancy.CanonicalURL
	if strings.TrimSpace(link) == "" {
		link = vacancy.SourceURL
	}
	lines := []string{
		"<b>" + html.EscapeString(vacancy.Title) + "</b>",
		html.EscapeString(company),
		"Source: " + html.EscapeString(hostnameFromURL(vacancy.SourceURL)),
		"Published: " + vacancy.PublishedAt.Format("2006-01-02"),
		"Format: " + html.EscapeString(vacancy.RemoteType),
		"Salary: " + html.EscapeString(salary),
		"Score: " + html.EscapeString(score),
	}
	if positive != "" {
		lines = append(lines, "Match: "+html.EscapeString(positive))
	}
	if negative != "" {
		lines = append(lines, "Risks: "+html.EscapeString(negative))
	}
	if link != "" {
		lines = append(lines, html.EscapeString(link))
	}
	return strings.Join(lines, "\n")
}

func salaryRangeLabel(from, to *int, currency string) string {
	if from == nil && to == nil {
		return "not specified"
	}
	switch {
	case from != nil && to != nil:
		return fmt.Sprintf("%d-%d %s", *from, *to, currency)
	case from != nil:
		return fmt.Sprintf("from %d %s", *from, currency)
	case to != nil:
		return fmt.Sprintf("to %d %s", *to, currency)
	default:
		return "not specified"
	}
}

func splitCommand(text string) (string, string) {
	text = strings.TrimSpace(text)
	if !strings.HasPrefix(text, "/") {
		return "", ""
	}
	fields := strings.Fields(text)
	command := strings.TrimPrefix(fields[0], "/")
	if idx := strings.Index(command, "@"); idx >= 0 {
		command = command[:idx]
	}
	arg := ""
	if len(fields) > 1 {
		arg = strings.Join(fields[1:], " ")
	}
	return strings.ToLower(command), arg
}

func sleepFor(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (a *App) isTelegramOwner(userID int64) bool {
	return userID == a.cfg.TelegramOwnerID
}
