package main

import (
	"fmt"
	"log"
	"os"
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
			tgbotapi.NewInlineKeyboardButtonData("📏 Подобрать размер", "start_survey"),
		),
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
	// Проверяем переменную окружения MANAGER_ID
	managerIDStr := os.Getenv("MANAGER_ID")

	// Если MANAGER_ID == "0" или пустой, то клиентский режим - менеджер не определен
	if managerIDStr == "0" || managerIDStr == "" {
		return false
	}

	// Если MANAGER_ID содержит реальный ID, проверяем по нему
	if managerID, err := strconv.ParseInt(managerIDStr, 10, 64); err == nil && message.From.ID == managerID {
		return true
	}

	// Проверяем по username (для автоматического определения) - только если MANAGER_ID не 0
	if message.From.UserName == "Shpinatyamba" {
		// Устанавливаем ID менеджера при первом сообщении
		if managerID == 0 {
			managerID = message.From.ID
			log.Printf("Автоматически установлен ID менеджера: %d (@%s)", managerID, message.From.UserName)
		}
		return true
	}

	return false
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
