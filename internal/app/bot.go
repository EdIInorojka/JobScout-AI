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
	parts := strings.SplitN(cb.Data, ":", 3)
	if len(parts) != 3 {
		_ = a.tgClient.AnswerCallbackQuery(ctx, cb.ID, "Unknown action")
		return nil
	}
	prefix := parts[0]
	id := parts[1]
	action := parts[2]
	switch prefix {
	case "vac":
		switch action {
		case "prep", "like":
			return a.handleTelegramPrepareApplication(ctx, cb, id)
		case "skip":
			item, err := a.UpdateVacancyStatus(ctx, id, core.VacancyStatusArchived)
			if err != nil {
				_ = a.tgClient.AnswerCallbackQuery(ctx, cb.ID, html.EscapeString(err.Error()))
				return err
			}
			if cb.Message != nil {
				edited := vacancyCardText(*item)
				_ = a.tgClient.EditMessageText(ctx, cb.Message.Chat.ID, cb.Message.MessageID, edited, vacancyCardMarkup(*item))
			}
			_ = a.tgClient.AnswerCallbackQuery(ctx, cb.ID, "Updated")
			return nil
		default:
			_ = a.tgClient.AnswerCallbackQuery(ctx, cb.ID, "Unknown action")
			return nil
		}
	case "app":
		switch action {
		case "approve":
			return a.handleTelegramApproveApplication(ctx, cb, id)
		case "cancel":
			return a.handleTelegramCancelApplication(ctx, cb, id)
		case "submitted":
			return a.handleTelegramSubmittedApplication(ctx, cb, id)
		case "hr":
			return a.handleTelegramOutcomeApplication(ctx, cb, id, core.ApplicationStatusHRContact)
		case "interview":
			return a.handleTelegramOutcomeApplication(ctx, cb, id, core.ApplicationStatusInterview)
		case "offer":
			return a.handleTelegramOutcomeApplication(ctx, cb, id, core.ApplicationStatusOffer)
		case "reject":
			return a.handleTelegramOutcomeApplication(ctx, cb, id, core.ApplicationStatusRejected)
		default:
			_ = a.tgClient.AnswerCallbackQuery(ctx, cb.ID, "Unknown action")
			return nil
		}
	default:
		_ = a.tgClient.AnswerCallbackQuery(ctx, cb.ID, "Unknown action")
		return nil
	}
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
	if item.Vacancy.Status == core.VacancyStatusArchived {
		if strings.TrimSpace(urlTarget) != "" {
			return tele.NewInlineKeyboard([]tele.InlineKeyboardButton{tele.URLButton("Открыть вакансию", urlTarget)})
		}
		return tele.NewInlineKeyboard()
	}
	rows := [][]tele.InlineKeyboardButton{
		{
			tele.Button("Подготовить отклик", "vac:"+item.Vacancy.ID+":prep"),
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

func (a *App) handleTelegramPrepareApplication(ctx context.Context, cb *tele.CallbackQuery, vacancyID string) error {
	view, err := a.PrepareApplication(ctx, vacancyID, nil, telegramActor(cb.From))
	if err != nil {
		_ = a.tgClient.AnswerCallbackQuery(ctx, cb.ID, html.EscapeString(err.Error()))
		return err
	}
	if cb.Message != nil {
		if err := a.tgClient.SendMessage(ctx, cb.Message.Chat.ID, applicationPreviewText(*view), applicationPreviewMarkup(*view)); err != nil {
			_ = a.tgClient.AnswerCallbackQuery(ctx, cb.ID, html.EscapeString(err.Error()))
			return err
		}
	}
	_ = a.tgClient.AnswerCallbackQuery(ctx, cb.ID, "Preview ready")
	return nil
}

func (a *App) handleTelegramApproveApplication(ctx context.Context, cb *tele.CallbackQuery, applicationID string) error {
	view, err := a.ApproveApplication(ctx, applicationID, telegramActor(cb.From))
	if err != nil {
		_ = a.tgClient.AnswerCallbackQuery(ctx, cb.ID, html.EscapeString(err.Error()))
		return err
	}
	return a.replyWithApplicationView(ctx, cb, view, "Approved")
}

func (a *App) handleTelegramCancelApplication(ctx context.Context, cb *tele.CallbackQuery, applicationID string) error {
	view, err := a.CancelApplication(ctx, applicationID, telegramActor(cb.From))
	if err != nil {
		_ = a.tgClient.AnswerCallbackQuery(ctx, cb.ID, html.EscapeString(err.Error()))
		return err
	}
	return a.replyWithApplicationView(ctx, cb, view, "Cancelled")
}

func (a *App) handleTelegramSubmittedApplication(ctx context.Context, cb *tele.CallbackQuery, applicationID string) error {
	view, err := a.MarkApplicationSubmitted(ctx, applicationID, telegramActor(cb.From))
	if err != nil {
		_ = a.tgClient.AnswerCallbackQuery(ctx, cb.ID, html.EscapeString(err.Error()))
		return err
	}
	return a.replyWithApplicationView(ctx, cb, view, "Saved")
}

func (a *App) handleTelegramOutcomeApplication(ctx context.Context, cb *tele.CallbackQuery, applicationID string, status core.ApplicationStatus) error {
	view, err := a.UpdateApplicationOutcome(ctx, applicationID, ApplicationOutcomeRequest{Status: string(status)}, telegramActor(cb.From))
	if err != nil {
		_ = a.tgClient.AnswerCallbackQuery(ctx, cb.ID, html.EscapeString(err.Error()))
		return err
	}
	return a.replyWithApplicationView(ctx, cb, view, "Saved")
}

func (a *App) replyWithApplicationView(ctx context.Context, cb *tele.CallbackQuery, view *ApplicationView, answerText string) error {
	text, markup := applicationStatusText(*view), applicationStatusMarkup(*view)
	if cb.Message == nil {
		_ = a.tgClient.AnswerCallbackQuery(ctx, cb.ID, answerText)
		return nil
	}
	_ = a.tgClient.EditMessageText(ctx, cb.Message.Chat.ID, cb.Message.MessageID, text, markup)
	_ = a.tgClient.AnswerCallbackQuery(ctx, cb.ID, answerText)
	return nil
}

func applicationPreviewText(view ApplicationView) string {
	vacancy := view.Vacancy
	company := ""
	if view.Company != nil {
		company = view.Company.DisplayName
	}
	score := "n/a"
	if view.Match != nil {
		score = strconv.Itoa(view.Match.TotalScore)
	}
	resumeLine := "n/a"
	if view.Resume != nil {
		resumeLine = fmt.Sprintf("%s (%s, %s)", html.EscapeString(view.Resume.Name), html.EscapeString(core.ResumeTargetRoleLabel(view.Resume.TargetRole)), html.EscapeString(string(view.Resume.Language)))
	}
	lines := []string{
		"<b>Подготовка отклика</b>",
		"<b>" + html.EscapeString(vacancy.Title) + "</b>",
		html.EscapeString(company),
		"Score: " + html.EscapeString(score),
		"Резюме: " + resumeLine,
		"<b>Сопроводительное письмо</b>",
		html.EscapeString(view.Application.CoverLetter),
	}
	if len(view.Warnings) > 0 {
		lines = append(lines, "<b>Warnings</b>")
		for _, warning := range view.Warnings {
			lines = append(lines, "• "+html.EscapeString(warning))
		}
	}
	if view.VacancyURL != "" {
		lines = append(lines, html.EscapeString(view.VacancyURL))
	}
	return strings.Join(lines, "\n")
}

func applicationStatusText(view ApplicationView) string {
	vacancy := view.Vacancy
	company := ""
	if view.Company != nil {
		company = view.Company.DisplayName
	}
	score := "n/a"
	if view.Match != nil {
		score = strconv.Itoa(view.Match.TotalScore)
	}
	resumeLine := "n/a"
	if view.Resume != nil {
		resumeLine = fmt.Sprintf("%s (%s, %s)", html.EscapeString(view.Resume.Name), html.EscapeString(core.ResumeTargetRoleLabel(view.Resume.TargetRole)), html.EscapeString(string(view.Resume.Language)))
	}
	status := view.Application.Status
	lines := []string{
		"<b>" + html.EscapeString(applicationStatusLabel(status)) + "</b>",
		"<b>" + html.EscapeString(vacancy.Title) + "</b>",
		html.EscapeString(company),
		"Score: " + html.EscapeString(score),
		"Резюме: " + resumeLine,
	}
	switch status {
	case core.ApplicationStatusManualActionRequired, core.ApplicationStatusApproved:
		lines = append(lines,
			"Автоматическая отправка не выполнена.",
			"Открой вакансию по ссылке и нажми «Я откликнулся» после ручной отправки.",
		)
	case core.ApplicationStatusSubmitted:
		lines = append(lines, "Ручная отправка зафиксирована. Теперь можно отмечать результат.")
	case core.ApplicationStatusCancelled:
		lines = append(lines, "Отклик отменён. При необходимости можно подготовить новый.")
	case core.ApplicationStatusHRContact, core.ApplicationStatusInterview, core.ApplicationStatusOffer, core.ApplicationStatusRejected:
		lines = append(lines, "Результат вручную зафиксирован.")
	default:
		lines = append(lines, "Черновик отклика сохранён.")
	}
	if view.VacancyURL != "" {
		lines = append(lines, html.EscapeString(view.VacancyURL))
	}
	return strings.Join(lines, "\n")
}

func applicationPreviewMarkup(view ApplicationView) *tele.InlineKeyboardMarkup {
	rows := [][]tele.InlineKeyboardButton{
		{
			tele.Button("Подтвердить", "app:"+view.Application.ID+":approve"),
			tele.Button("Отменить", "app:"+view.Application.ID+":cancel"),
		},
	}
	if strings.TrimSpace(view.VacancyURL) != "" {
		rows = append(rows, []tele.InlineKeyboardButton{tele.URLButton("Открыть вакансию", view.VacancyURL)})
	}
	return tele.NewInlineKeyboard(rows...)
}

func applicationStatusMarkup(view ApplicationView) *tele.InlineKeyboardMarkup {
	switch view.Application.Status {
	case core.ApplicationStatusManualActionRequired, core.ApplicationStatusApproved:
		rows := [][]tele.InlineKeyboardButton{
			{tele.Button("Я откликнулся", "app:"+view.Application.ID+":submitted")},
		}
		if strings.TrimSpace(view.VacancyURL) != "" {
			rows = append(rows, []tele.InlineKeyboardButton{tele.URLButton("Открыть вакансию", view.VacancyURL)})
		}
		return tele.NewInlineKeyboard(rows...)
	case core.ApplicationStatusSubmitted, core.ApplicationStatusHRContact, core.ApplicationStatusInterview, core.ApplicationStatusOffer, core.ApplicationStatusRejected:
		rows := [][]tele.InlineKeyboardButton{
			{
				tele.Button("Связался HR", "app:"+view.Application.ID+":hr"),
				tele.Button("Назначено интервью", "app:"+view.Application.ID+":interview"),
			},
			{
				tele.Button("Получен отказ", "app:"+view.Application.ID+":reject"),
				tele.Button("Получен оффер", "app:"+view.Application.ID+":offer"),
			},
		}
		if strings.TrimSpace(view.VacancyURL) != "" {
			rows = append(rows, []tele.InlineKeyboardButton{tele.URLButton("Открыть вакансию", view.VacancyURL)})
		}
		return tele.NewInlineKeyboard(rows...)
	case core.ApplicationStatusCancelled:
		rows := [][]tele.InlineKeyboardButton{}
		if view.Vacancy != nil {
			rows = append(rows, []tele.InlineKeyboardButton{tele.Button("Подготовить отклик", "vac:"+view.Vacancy.ID+":prep")})
		}
		if strings.TrimSpace(view.VacancyURL) != "" {
			rows = append(rows, []tele.InlineKeyboardButton{tele.URLButton("Открыть вакансию", view.VacancyURL)})
		}
		return tele.NewInlineKeyboard(rows...)
	default:
		return applicationPreviewMarkup(view)
	}
}

func applicationStatusLabel(status core.ApplicationStatus) string {
	switch status {
	case core.ApplicationStatusDraft:
		return "Черновик отклика"
	case core.ApplicationStatusWaitingApproval:
		return "Отклик готов"
	case core.ApplicationStatusApproved:
		return "Отклик подтверждён"
	case core.ApplicationStatusManualActionRequired:
		return "Нужна ручная отправка"
	case core.ApplicationStatusSubmitted:
		return "Отклик отправлен вручную"
	case core.ApplicationStatusCancelled:
		return "Отклик отменён"
	case core.ApplicationStatusHRContact:
		return "HR связался"
	case core.ApplicationStatusInterview:
		return "Назначено интервью"
	case core.ApplicationStatusOffer:
		return "Получен оффер"
	case core.ApplicationStatusRejected:
		return "Получен отказ"
	default:
		return string(status)
	}
}

func telegramActor(user tele.User) string {
	return firstNonEmpty(user.FirstName, user.Username, fmt.Sprintf("telegram:%d", user.ID))
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
