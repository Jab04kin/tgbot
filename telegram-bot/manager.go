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
			tgbotapi.NewInlineKeyboardButtonData("❓ Помощь", "help"),
		),
	)

	msg.ReplyMarkup = keyboard
	bot.Send(msg)
}

func handleManagerTicketsCallback(bot *tgbotapi.BotAPI, chatID int64) {
	if len(tickets) == 0 {
		msg := tgbotapi.NewMessage(chatID, "📭 Нет тикетов")

		keyboard := tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("🔙 Назад", "back_to_manager_menu"),
			),
		)
		msg.ReplyMarkup = keyboard
		bot.Send(msg)
		return
	}

	// Показываем тикеты по 5 штук с кнопками
	showTicketsWithButtons(bot, chatID, tickets, "🎫 Все тикеты")
}

func handleManagerOpenTicketsCallback(bot *tgbotapi.BotAPI, chatID int64) {
	openTickets := make(map[int]*Ticket)
	for _, ticket := range tickets {
		if ticket.Status == "open" {
			openTickets[ticket.ID] = ticket
		}
	}

	if len(openTickets) == 0 {
		msg := tgbotapi.NewMessage(chatID, "📭 Нет открытых тикетов")

		keyboard := tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("🔙 Назад", "back_to_manager_menu"),
			),
		)
		msg.ReplyMarkup = keyboard
		bot.Send(msg)
		return
	}

	showTicketsWithButtons(bot, chatID, openTickets, "🆕 Открытые тикеты")
}

func handleManagerClosedTicketsCallback(bot *tgbotapi.BotAPI, chatID int64) {
	closedTickets := make(map[int]*Ticket)
	for _, ticket := range tickets {
		if ticket.Status == "closed" {
			closedTickets[ticket.ID] = ticket
		}
	}

	if len(closedTickets) == 0 {
		msg := tgbotapi.NewMessage(chatID, "📭 Нет закрытых тикетов")

		keyboard := tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("🔙 Назад", "back_to_manager_menu"),
			),
		)
		msg.ReplyMarkup = keyboard
		bot.Send(msg)
		return
	}

	showTicketsWithButtons(bot, chatID, closedTickets, "🔴 Закрытые тикеты")
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

func showAdminPanel(bot *tgbotapi.BotAPI, chatID int64) {
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
			tgbotapi.NewInlineKeyboardButtonData("🔙 Назад", "back_to_menu"),
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
