package main

import (
	"fmt"
	"log"
	"strconv"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func sendManagerMenu(bot *tgbotapi.BotAPI, chatID int64) {
	// Подсчитываем статистику тикетов
	openTickets := 0
	closedTickets := 0
	for _, ticket := range tickets {
		if ticket.Status == "open" {
			openTickets++
		} else {
			closedTickets++
		}
	}

	msg := tgbotapi.NewMessage(chatID, fmt.Sprintf("👨‍💼 Добро пожаловать, менеджер!\n\n📊 Тикеты: 🟢 %d открытых | 🔴 %d закрытых\n\nВыберите действие:", openTickets, closedTickets))

	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("📚 Каталог", "catalog"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("👥 Клиенты", "manager_tickets"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("📊 Статистика", "manager_export_menu"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("❓ Помощь", "help"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🚪 Выйти из панели менеджера", "exit_manager_panel"),
		),
	)

	msg.ReplyMarkup = keyboard
	bot.Send(msg)
}

func handleManagerTicketsCallback(bot *tgbotapi.BotAPI, chatID int64) {
	showTicketsWithFilters(bot, chatID)
}

func handleManagerOpenTicketsCallback(bot *tgbotapi.BotAPI, chatID int64) {
	showTicketsWithFilters(bot, chatID, "open")
}

func handleManagerClosedTicketsCallback(bot *tgbotapi.BotAPI, chatID int64) {
	showTicketsWithFilters(bot, chatID, "closed")
}

func handleManagerStatsCallback(bot *tgbotapi.BotAPI, chatID int64) {
	totalTickets := len(tickets)
	openTickets := 0
	closedTickets := 0

	for _, ticket := range tickets {
		if ticket.Status == "open" {
			openTickets++
		} else {
			closedTickets++
		}
	}

	msg := tgbotapi.NewMessage(chatID, fmt.Sprintf("📊 Статистика тикетов:\n\n"+
		"📈 Всего тикетов: %d\n"+
		"🟢 Открытых: %d\n"+
		"🔴 Закрытых: %d\n"+
		"📅 Последний ID: %d",
		totalTickets, openTickets, closedTickets, nextTicketID-1))

	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🔙 Назад", "back_to_manager_menu"),
		),
	)
	msg.ReplyMarkup = keyboard
	bot.Send(msg)
}

func handleManagerHelpCallback(bot *tgbotapi.BotAPI, chatID int64) {
	msg := tgbotapi.NewMessage(chatID, "❓ Помощь менеджеру:\n\n"+
		"🔘 Доступные кнопки:\n"+
		"• Список тикетов - показать все тикеты\n"+
		"• Новые тикеты - показать количество открытых\n"+
		"• Статистика - общая статистика\n"+
		"• Помощь - эта справка\n\n"+
		"💡 Все действия выполняются через кнопки для удобства управления")

	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🔙 Назад", "back_to_manager_menu"),
		),
	)
	msg.ReplyMarkup = keyboard
	bot.Send(msg)
}

// Меню экспорта статистики
func handleManagerExportMenu(bot *tgbotapi.BotAPI, chatID int64) {
	msg := tgbotapi.NewMessage(chatID, "📊 Экспорт статистики:\n\nВыберите формат и данные для выгрузки:")
	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("1) Пользователи (Excel)", "manager_export_users"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("1) Пользователи (XML)", "manager_export_users_xml"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("2) Все тикеты", "manager_export_tickets"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("3) Тикет по ID", "manager_export_ticket_by_id"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🔙 Назад", "back_to_manager_menu"),
		),
	)
	msg.ReplyMarkup = keyboard
	bot.Send(msg)
}

var exportTicketIDState = make(map[int64]bool) // chatID -> ждем ID тикета для экспорта

// Очищает состояния клиента (не трогает привязку userTickets)
func clearClientStates(chatID int64) {
	delete(userStates, chatID)
	delete(questionStates, chatID)
	delete(messageModeStates, chatID)
	delete(searchState, chatID)
	delete(exportTicketIDState, chatID)
}

// Очищает состояния менеджера и разрывает режим ответа (удаляет userTickets для чата менеджера)
func clearManagerStates(chatID int64) {
	clearClientStates(chatID)
	delete(userTickets, chatID)
}

func isManagerResponse(message *tgbotapi.Message) bool {
	return isManagerUser(message.From)
}

func handleManagerResponse(bot *tgbotapi.BotAPI, message *tgbotapi.Message) {
	text := message.Text

	// Проверяем, находится ли менеджер в режиме ответа на тикет
	if ticketID, exists := userTickets[message.Chat.ID]; exists {
		handleManagerReplyToTicket(bot, message, ticketID)
		return
	}

	// Обработка команд менеджера
	switch {
	case strings.HasPrefix(text, "Ответ:"):
		handleOldReplyFormat(bot, message)
	default:
		// Показываем меню с кнопками
		sendManagerMenu(bot, message.Chat.ID)
	}
}

func handleOldReplyFormat(bot *tgbotapi.BotAPI, message *tgbotapi.Message) {
	// Старый формат для обратной совместимости
	parts := strings.SplitN(message.Text, " ", 3)
	if len(parts) >= 3 {
		userID, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil {
			msg := tgbotapi.NewMessage(message.Chat.ID, "❌ Неверный формат. Используйте: Ответ: [ID_пользователя] [текст_ответа]")
			bot.Send(msg)
			return
		}

		answerText := parts[2]
		responseMsg := tgbotapi.NewMessage(userID, fmt.Sprintf("💬 Ответ от менеджера:\n\n%s", answerText))
		bot.Send(responseMsg)

		confirmMsg := tgbotapi.NewMessage(message.Chat.ID, fmt.Sprintf("✅ Ответ отправлен пользователю %d", userID))
		bot.Send(confirmMsg)

		log.Printf("Менеджер ответил пользователю %d: %s", userID, answerText)
	} else {
		msg := tgbotapi.NewMessage(message.Chat.ID, "❌ Неверный формат. Используйте: Ответ: [ID_пользователя] [текст_ответа]")
		bot.Send(msg)
	}
}

// ===== Админ-панель =====

var adminActionState = make(map[int64]string) // chatID -> "add_manager" | "remove_manager"
var searchState = make(map[int64]bool)        // chatID -> true если в режиме поиска тикета

func showAdminPanel(bot *tgbotapi.BotAPI, chatID int64) {
	clearManagerStates(chatID)
	msg := tgbotapi.NewMessage(chatID, "⚙️ Админ-панель")
	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("👥 Список менеджеров", "admin_list_managers"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("➕ Назначить менеджера", "admin_add_manager"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("➖ Снять менеджера", "admin_remove_manager"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🔙 Назад", "back_to_manager_menu"),
		),
	)
	msg.ReplyMarkup = keyboard
	bot.Send(msg)
}

func showManagersList(bot *tgbotapi.BotAPI, chatID int64) {
	ids := getManagerIDs()
	if len(ids) == 0 && len(managerUsernamesSet) == 0 {
		bot.Send(tgbotapi.NewMessage(chatID, "Менеджеры не заданы"))
		return
	}
	var b strings.Builder
	b.WriteString("Текущие менеджеры:\n")
	for _, id := range ids {
		b.WriteString(fmt.Sprintf("• ID: %d\n", id))
	}
	for u := range managerUsernamesSet {
		b.WriteString(fmt.Sprintf("• @%s (по username)\n", u))
	}
	bot.Send(tgbotapi.NewMessage(chatID, b.String()))
}

func promptAddManager(bot *tgbotapi.BotAPI, chatID int64) {
	adminActionState[chatID] = "add_manager"
	bot.Send(tgbotapi.NewMessage(chatID, "Отправьте:\n• форвард сообщения пользователя\n• или его числовой ID\n• или @username"))
}

func promptRemoveManager(bot *tgbotapi.BotAPI, chatID int64) {
	adminActionState[chatID] = "remove_manager"
	bot.Send(tgbotapi.NewMessage(chatID, "Кого снять? Пришлите форвард, числовой ID или @username"))
}

func handleAdminInput(bot *tgbotapi.BotAPI, message *tgbotapi.Message) bool {
	chatID := message.Chat.ID
	action, has := adminActionState[chatID]
	if !has {
		return false
	}

	// Пытаемся извлечь пользователя из форварда
	var targetID int64
	var targetUsername string
	if message.ForwardFrom != nil {
		targetID = message.ForwardFrom.ID
		targetUsername = message.ForwardFrom.UserName
	} else {
		text := strings.TrimSpace(message.Text)
		if strings.HasPrefix(text, "@") {
			targetUsername = strings.TrimPrefix(text, "@")
		} else if id, err := strconv.ParseInt(text, 10, 64); err == nil {
			targetID = id
		} else {
			bot.Send(tgbotapi.NewMessage(chatID, "Не удалось распознать пользователя. Пришлите форвард, ID или @username"))
			return true
		}
	}

	switch action {
	case "add_manager":
		if targetID != 0 {
			addManagerByID(targetID)
		}
		if targetUsername != "" {
			managerUsernamesSet[strings.ToLower(targetUsername)] = true
			saveManagersToFile()
		}
		// Уведомления
		bot.Send(tgbotapi.NewMessage(chatID, fmt.Sprintf("✅ Назначен менеджером: %s%v",
			usernameFmt(targetUsername), idFmt(targetID))))
		if targetID != 0 {
			bot.Send(tgbotapi.NewMessage(targetID, "✅ Вы назначены менеджером"))
		}
	case "remove_manager":
		changed := false
		if targetID != 0 {
			if managerIDsSet[targetID] {
				removeManagerByID(targetID)
				changed = true
			}
		}
		if targetUsername != "" {
			lu := strings.ToLower(targetUsername)
			if managerUsernamesSet[lu] {
				delete(managerUsernamesSet, lu)
				saveManagersToFile()
				changed = true
			}
		}
		if changed {
			bot.Send(tgbotapi.NewMessage(chatID, fmt.Sprintf("✅ Снят с менеджеров: %s%v",
				usernameFmt(targetUsername), idFmt(targetID))))
			if targetID != 0 {
				bot.Send(tgbotapi.NewMessage(targetID, "⚠️ Вы больше не менеджер"))
			}
		} else {
			bot.Send(tgbotapi.NewMessage(chatID, "Пользователь не найден среди менеджеров"))
		}
	}

	delete(adminActionState, chatID)
	return true
}

func usernameFmt(u string) string {
	if u == "" {
		return ""
	}
	return "@" + u + " "
}

func idFmt(id int64) string {
	if id == 0 {
		return ""
	}
	return fmt.Sprintf("(ID %d)", id)
}

// showTicketsWithFilters показывает тикеты с фильтрацией и поиском
func showTicketsWithFilters(bot *tgbotapi.BotAPI, chatID int64, statusFilter ...string) {
	var filteredTickets []*Ticket
	var title string

	// Фильтрация по статусу
	if len(statusFilter) > 0 && statusFilter[0] != "" {
		for _, ticket := range tickets {
			if ticket.Status == statusFilter[0] {
				filteredTickets = append(filteredTickets, ticket)
			}
		}
		switch statusFilter[0] {
		case "open":
			title = "🆕 Открытые тикеты"
		case "closed":
			title = "🔴 Закрытые тикеты"
		default:
			title = "🎫 Тикеты"
		}
	} else {
		for _, ticket := range tickets {
			filteredTickets = append(filteredTickets, ticket)
		}
		title = "🎫 Все тикеты"
	}

	// Сортировка по ID (новые сверху)
	for i := 0; i < len(filteredTickets)-1; i++ {
		for j := i + 1; j < len(filteredTickets); j++ {
			if filteredTickets[i].ID < filteredTickets[j].ID {
				filteredTickets[i], filteredTickets[j] = filteredTickets[j], filteredTickets[i]
			}
		}
	}

	// Формируем сообщение
	var text strings.Builder
	text.WriteString(fmt.Sprintf("%s (%d):\n\n", title, len(filteredTickets)))

	if len(filteredTickets) == 0 {
		text.WriteString("📭 Нет тикетов")
	} else {
		// Показываем первые 10 тикетов с подробной информацией
		limit := 10
		if len(filteredTickets) < limit {
			limit = len(filteredTickets)
		}

		for i := 0; i < limit; i++ {
			ticket := filteredTickets[i]
			status := "🟢"
			if ticket.Status == "closed" {
				status = "🔴"
			}

			// Формируем имя пользователя
			name := strings.TrimSpace(ticket.FirstName + " " + ticket.LastName)
			if name == "" {
				name = "Без имени"
			}

			// Формируем username
			username := ""
			if ticket.Username != "" {
				username = fmt.Sprintf(" (@%s)", ticket.Username)
			}

			// Время создания
			timeStr := ticket.CreatedAt.Format("02.01 15:04")

			text.WriteString(fmt.Sprintf("%s #%d %s%s\n🆔 %d | %s\n\n",
				status, ticket.ID, name, username, ticket.UserID, timeStr))
		}

		if len(filteredTickets) > 10 {
			text.WriteString(fmt.Sprintf("... и еще %d тикетов", len(filteredTickets)-10))
		}
	}

	msg := tgbotapi.NewMessage(chatID, text.String())

	// Создаем кнопки
	var keyboard [][]tgbotapi.InlineKeyboardButton

	// Кнопки фильтров
	keyboard = append(keyboard, []tgbotapi.InlineKeyboardButton{
		tgbotapi.NewInlineKeyboardButtonData("🎫 Все", "manager_tickets"),
		tgbotapi.NewInlineKeyboardButtonData("🆕 Открытые", "manager_open_tickets"),
		tgbotapi.NewInlineKeyboardButtonData("🔴 Закрытые", "manager_closed_tickets"),
	})

	// Кнопка поиска
	keyboard = append(keyboard, []tgbotapi.InlineKeyboardButton{
		tgbotapi.NewInlineKeyboardButtonData("🔍 Поиск по номеру", "manager_search_ticket"),
	})

	// Кнопки тикетов (максимум 5 в ряд)
	buttonLimit := 10
	if len(filteredTickets) < buttonLimit {
		buttonLimit = len(filteredTickets)
	}
	for i := 0; i < buttonLimit && i < len(filteredTickets); i++ {
		ticketID := filteredTickets[i].ID
		ticket := filteredTickets[i]

		buttonText := fmt.Sprintf("#%d", ticketID)
		if len(ticket.FirstName) > 0 {
			shortName := ticket.FirstName
			if len(shortName) > 8 {
				shortName = shortName[:8] + "..."
			}
			buttonText = fmt.Sprintf("#%d %s", ticketID, shortName)
		}

		button := tgbotapi.NewInlineKeyboardButtonData(buttonText, fmt.Sprintf("ticket_view_%d", ticketID))

		// Добавляем кнопку в ряд
		if len(keyboard) == 0 || len(keyboard[len(keyboard)-1]) >= 2 {
			keyboard = append(keyboard, []tgbotapi.InlineKeyboardButton{button})
		} else {
			keyboard[len(keyboard)-1] = append(keyboard[len(keyboard)-1], button)
		}
	}

	// Кнопка "Назад"
	keyboard = append(keyboard, []tgbotapi.InlineKeyboardButton{
		tgbotapi.NewInlineKeyboardButtonData("🔙 Назад", "back_to_manager_menu"),
	})

	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(keyboard...)
	bot.Send(msg)
}

// handleManagerSearchTicket обрабатывает поиск тикета по номеру
func handleManagerSearchTicket(bot *tgbotapi.BotAPI, chatID int64) {
	searchState[chatID] = true
	msg := tgbotapi.NewMessage(chatID, "🔍 Введите номер тикета для поиска:\n\nИспользуйте /cancel для отмены")

	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("❌ Отмена", "manager_tickets"),
		),
	)
	msg.ReplyMarkup = keyboard
	bot.Send(msg)
}

// handleTicketSearchInput обрабатывает ввод номера тикета для поиска
func handleTicketSearchInput(bot *tgbotapi.BotAPI, message *tgbotapi.Message) bool {
	chatID := message.Chat.ID
	if !searchState[chatID] {
		return false
	}

	if message.Text == "/cancel" {
		delete(searchState, chatID)
		showTicketsWithFilters(bot, chatID)
		return true
	}

	// Парсим номер тикета
	ticketID, err := strconv.Atoi(strings.TrimSpace(message.Text))
	if err != nil {
		bot.Send(tgbotapi.NewMessage(chatID, "❌ Неверный формат. Введите числовой номер тикета или /cancel"))
		return true
	}

	// Ищем тикет
	_, exists := tickets[ticketID]
	if !exists {
		bot.Send(tgbotapi.NewMessage(chatID, fmt.Sprintf("❌ Тикет #%d не найден", ticketID)))
		delete(searchState, chatID)
		return true
	}

	// Показываем найденный тикет
	delete(searchState, chatID)
	showTicketDetails(bot, chatID, ticketID)
	return true
}

// handleExportTicketIDInput обрабатывает ввод ID тикета для экспорта
func handleExportTicketIDInput(bot *tgbotapi.BotAPI, message *tgbotapi.Message) bool {
	chatID := message.Chat.ID
	if !exportTicketIDState[chatID] {
		return false
	}
	if message.Text == "/cancel" {
		delete(exportTicketIDState, chatID)
		handleManagerExportMenu(bot, chatID)
		return true
	}
	id, err := strconv.Atoi(strings.TrimSpace(message.Text))
	if err != nil {
		bot.Send(tgbotapi.NewMessage(chatID, "❌ Неверный формат. Введите числовой ID тикета или /cancel"))
		return true
	}
	if _, ok := tickets[id]; !ok {
		bot.Send(tgbotapi.NewMessage(chatID, fmt.Sprintf("❌ Тикет #%d не найден", id)))
		delete(exportTicketIDState, chatID)
		return true
	}
	if buf, err := exportSingleTicketExcel(id); err == nil {
		sendExcelBuffer(bot, chatID, fmt.Sprintf("ticket_%d.xlsx", id), buf)
	} else {
		bot.Send(tgbotapi.NewMessage(chatID, "❌ Ошибка формирования файла"))
	}
	delete(exportTicketIDState, chatID)
	return true
}

// notifyNewUserWithAssign отправляет админам уведомление о новом пользователе с кнопкой "Назначить менеджером"
func notifyNewUserWithAssign(bot *tgbotapi.BotAPI, user *tgbotapi.User) {
	if user == nil {
		return
	}

	name := strings.TrimSpace(strings.TrimSpace(user.FirstName + " " + user.LastName))
	if name == "" {
		name = "(без имени)"
	}
	uname := user.UserName
	if uname == "" {
		uname = "—"
	} else {
		uname = "@" + uname
	}

	text := fmt.Sprintf("🆕 Новый пользователь написал боту\nИмя: %s\nUsername: %s\nID: %d", name, uname, user.ID)

	label := "➕ Назначить менеджером"
	if user.UserName != "" {
		label = fmt.Sprintf("➕ Назначить %s менеджером", "@"+user.UserName)
	}

	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(
				label,
				fmt.Sprintf("admin_assign_manager_id_%d", user.ID),
			),
		),
	)

	for _, aid := range getAdminIDs() {
		if aid == 0 {
			continue
		}
		msg := tgbotapi.NewMessage(aid, text)
		msg.ReplyMarkup = keyboard
		bot.Send(msg)
	}
}
