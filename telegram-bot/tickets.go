package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Глобальные переменные для работы с тикетами
var tickets = make(map[int]*Ticket)   // все тикеты
var userTickets = make(map[int64]int) // связь пользователь -> ID тикета
var nextTicketID = 1
var managerID int64 = 0 // @Shpinatyamba - будет установлен автоматически при первом сообщении

type Message struct {
	ID            int       `json:"id"`
	SenderID      int64     `json:"sender_id"`
	Text          string    `json:"text"`
	Time          time.Time `json:"time"`
	IsFromManager bool      `json:"is_from_manager"`
}

type Ticket struct {
	ID              int       `json:"id"`
	UserID          int64     `json:"user_id"`
	Username        string    `json:"username"`
	FirstName       string    `json:"first_name"`
	LastName        string    `json:"last_name"`
	Height          int       `json:"height"`
	ChestSize       int       `json:"chest_size"`
	Oversize        bool      `json:"oversize"`
	RecommendedSize string    `json:"recommended_size"`
	Question        string    `json:"question"`
	Status          string    `json:"status"` // "open", "closed"
	CreatedAt       time.Time `json:"created_at"`
	LastMessage     time.Time `json:"last_message"`
	Messages        []Message `json:"messages"`
}

// Функции для работы с файлом тикетов
func saveTickets() {
	data, err := json.MarshalIndent(tickets, "", "  ")
	if err != nil {
		log.Printf("Ошибка сериализации тикетов: %v", err)
		return
	}

	err = os.WriteFile("tickets.json", data, 0644)
	if err != nil {
		log.Printf("Ошибка сохранения тикетов: %v", err)
	} else {
		log.Printf("Тикеты сохранены в файл")
	}
}

func loadTickets() {
	data, err := os.ReadFile("tickets.json")
	if err != nil {
		log.Printf("Файл тикетов не найден, начинаем с пустого списка")
		return
	}

	// Проверяем, является ли файл пустым или содержит пустой массив
	dataStr := strings.TrimSpace(string(data))
	if dataStr == "" || dataStr == "[]" {
		log.Printf("Файл тикетов пустой, начинаем с пустого списка")
		return
	}

	err = json.Unmarshal(data, &tickets)
	if err != nil {
		log.Printf("Ошибка загрузки тикетов: %v", err)
		return
	}

	// Восстанавливаем nextTicketID
	maxID := 0
	for id := range tickets {
		if id > maxID {
			maxID = id
		}
	}
	nextTicketID = maxID + 1

	// Восстанавливаем userTickets
	for id, ticket := range tickets {
		userTickets[ticket.UserID] = id
	}

	log.Printf("Загружено %d тикетов из файла", len(tickets))
}

func createTicketAndAskQuestion(bot *tgbotapi.BotAPI, chatID int64, recommendedSize string) {
	// Проверяем, есть ли данные пользователя для создания тикета
	state, exists := userStates[chatID]

	// Создаем тикет с данными клиента (если есть) или без них
	var ticket *Ticket
	now := time.Now()

	if exists {
		// Есть данные подбора размера
		ticket = &Ticket{
			ID:              nextTicketID,
			UserID:          chatID,
			Username:        "", // будет заполнено при первом сообщении
			FirstName:       "", // будет заполнено при первом сообщении
			LastName:        "", // будет заполнено при первом сообщении
			Height:          state.Height,
			ChestSize:       state.ChestSize,
			Oversize:        state.Oversize,
			RecommendedSize: recommendedSize,
			Question:        "",
			Status:          "open",
			CreatedAt:       now,
			LastMessage:     now,
			Messages:        []Message{},
		}
	} else {
		// Нет данных подбора размера - создаем тикет без них
		ticket = &Ticket{
			ID:              nextTicketID,
			UserID:          chatID,
			Username:        "", // будет заполнено при первом сообщении
			FirstName:       "", // будет заполнено при первом сообщении
			LastName:        "", // будет заполнено при первом сообщении
			Height:          0,
			ChestSize:       0,
			Oversize:        false,
			RecommendedSize: recommendedSize,
			Question:        "",
			Status:          "open",
			CreatedAt:       now,
			LastMessage:     now,
			Messages:        []Message{},
		}
	}

	// Сохраняем тикет
	tickets[nextTicketID] = ticket
	userTickets[chatID] = nextTicketID
	nextTicketID++

	// Сохраняем тикеты в файл
	saveTickets()

	// Отправляем карточку клиента менеджеру
	sendClientCardToManager(bot, ticket)

	// Просим пользователя написать вопрос
	msg := tgbotapi.NewMessage(chatID, "✅ Создан диалог с менеджером!\n\nКакой у вас вопрос? Напишите его в этом чате, и менеджер получит ваше сообщение.")

	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Главное меню", "back_to_menu"),
		),
	)

	msg.ReplyMarkup = keyboard
	bot.Send(msg)

	// Включаем режим диалога с менеджером
	questionStates[chatID] = true
}

// Функции для работы с сообщениями в тикетах

// addMessageToTicket добавляет сообщение в тикет
func addMessageToTicket(ticketID int, senderID int64, text string, isFromManager bool) {
	ticket, exists := tickets[ticketID]
	if !exists {
		log.Printf("Тикет #%d не найден", ticketID)
		return
	}

	messageID := len(ticket.Messages) + 1
	message := Message{
		ID:            messageID,
		SenderID:      senderID,
		Text:          text,
		Time:          time.Now(),
		IsFromManager: isFromManager,
	}

	ticket.Messages = append(ticket.Messages, message)
	ticket.LastMessage = time.Now()

	saveTickets()
	log.Printf("Сообщение добавлено в тикет #%d", ticketID)
}

// updateTicketUserInfo обновляет информацию о пользователе в тикете
func updateTicketUserInfo(ticketID int, username, firstName, lastName string) {
	ticket, exists := tickets[ticketID]
	if !exists {
		log.Printf("Тикет #%d не найден для обновления информации", ticketID)
		return
	}

	ticket.Username = username
	ticket.FirstName = firstName
	ticket.LastName = lastName

	saveTickets()
	log.Printf("Информация пользователя обновлена в тикете #%d", ticketID)
}

// getTicketMessages возвращает все сообщения тикета в читаемом формате
func getTicketMessages(ticketID int) string {
	ticket, exists := tickets[ticketID]
	if !exists {
		return "Тикет не найден"
	}

	if len(ticket.Messages) == 0 {
		return "Сообщений пока нет"
	}

	var result strings.Builder
	result.WriteString(fmt.Sprintf("📋 Диалог тикета #%d:\n\n", ticketID))

	for _, msg := range ticket.Messages {
		senderType := "👤 Клиент"
		if msg.IsFromManager {
			senderType = "👨‍💼 Менеджер"
		}

		result.WriteString(fmt.Sprintf("%s (%s):\n%s\n\n",
			senderType,
			msg.Time.Format("02.01.2006 15:04:05"),
			msg.Text))
	}

	return result.String()
}

func sendClientCardToManager(bot *tgbotapi.BotAPI, ticket *Ticket) {
	oversizeText := "Нет"
	if ticket.Oversize {
		oversizeText = "Да"
	}

	// Формируем сообщение в зависимости от наличия данных
	var messageText string
	if ticket.Height > 0 && ticket.ChestSize > 0 {
		// Есть данные подбора размера
		messageText = fmt.Sprintf("🎫 Новый тикет #%d\n\n"+
			"👤 Клиент: %s %s (@%s)\n"+
			"🆔 ID: %d\n"+
			"📏 Рост: %d см\n"+
			"📐 Обхват груди: %d см\n"+
			"👕 Оверсайз: %s\n"+
			"✅ Рекомендуемый размер: %s\n"+
			"🕐 Создан: %s\n\n"+
			"💬 Ответьте клиенту текстом или используйте кнопки для управления тикетом",
			ticket.ID,
			ticket.FirstName,
			ticket.LastName,
			ticket.Username,
			ticket.UserID,
			ticket.Height,
			ticket.ChestSize,
			oversizeText,
			ticket.RecommendedSize,
			ticket.CreatedAt.Format("15:04 02.01.2006"))
	} else {
		// Нет данных подбора размера
		messageText = fmt.Sprintf("🎫 Новый тикет #%d\n\n"+
			"👤 Клиент: %s %s (@%s)\n"+
			"🆔 ID: %d\n"+
			"📏 Рост: Не указан\n"+
			"📐 Обхват груди: Не указан\n"+
			"👕 Оверсайз: Не указан\n"+
			"✅ Рекомендуемый размер: %s\n"+
			"🕐 Создан: %s\n\n"+
			"💬 Ответьте клиенту текстом или используйте кнопки для управления тикетом",
			ticket.ID,
			ticket.FirstName,
			ticket.LastName,
			ticket.Username,
			ticket.UserID,
			ticket.RecommendedSize,
			ticket.CreatedAt.Format("15:04 02.01.2006"))
	}

	// Получаем ID менеджера из переменной окружения
	managerIDStr := os.Getenv("MANAGER_ID")
	if managerIDStr == "0" || managerIDStr == "" {
		log.Printf("Менеджер не определен (клиентский режим), сообщение не отправлено")
		return
	}

	managerID, err := strconv.ParseInt(managerIDStr, 10, 64)
	if err != nil {
		log.Printf("Ошибка парсинга MANAGER_ID: %v", err)
		return
	}

	msg := tgbotapi.NewMessage(managerID, messageText)
	bot.Send(msg)

	log.Printf("Отправлена карточка клиента для тикета #%d менеджеру", ticket.ID)
}

func handleManagerQuestion(bot *tgbotapi.BotAPI, message *tgbotapi.Message) {
	chatID := message.Chat.ID
	question := message.Text

	// Находим тикет пользователя
	ticketID, exists := userTickets[chatID]
	if !exists {
		msg := tgbotapi.NewMessage(chatID, "❌ Тикет не найден. Создайте новый диалог с менеджером.")
		bot.Send(msg)
		delete(questionStates, chatID)
		return
	}

	ticket, found := tickets[ticketID]
	if !found || ticket.Status != "open" {
		msg := tgbotapi.NewMessage(chatID, "❌ Тикет закрыт или не найден. Создайте новый диалог с менеджером.")
		bot.Send(msg)
		delete(questionStates, chatID)
		return
	}

	// Обновляем данные пользователя в тикете
	updateTicketUserInfo(ticketID, message.From.UserName, message.From.FirstName, message.From.LastName)

	// Добавляем сообщение клиента в тикет
	addMessageToTicket(ticketID, chatID, question, false)

	// Отправляем сообщение менеджеру
	messageText := fmt.Sprintf("💬 Новое сообщение от клиента (тикет #%d):\n\n%s", ticketID, question)

	// Получаем ID менеджера из переменной окружения
	managerIDStr := os.Getenv("MANAGER_ID")
	if managerIDStr == "0" || managerIDStr == "" {
		log.Printf("Менеджер не определен (клиентский режим), сообщение не отправлено")
		return
	}

	managerID, err := strconv.ParseInt(managerIDStr, 10, 64)
	if err != nil {
		log.Printf("Ошибка парсинга MANAGER_ID: %v", err)
		return
	}

	msg := tgbotapi.NewMessage(managerID, messageText)
	bot.Send(msg)

	log.Printf("Отправлено сообщение от пользователя %d в тикет #%d", chatID, ticketID)
}

// showManagerTicketDialog показывает полный диалог тикета менеджеру
func showManagerTicketDialog(bot *tgbotapi.BotAPI, chatID int64, ticketID int) {
	ticket, exists := tickets[ticketID]
	if !exists {
		msg := tgbotapi.NewMessage(chatID, "❌ Тикет не найден")
		bot.Send(msg)
		return
	}

	if len(ticket.Messages) == 0 {
		msg := tgbotapi.NewMessage(chatID, "📋 Диалог пуст")
		bot.Send(msg)
		return
	}

	// Получаем полный диалог
	dialogText := getTicketMessages(ticketID)

	// Разбиваем на части если слишком длинное
	const maxMessageLength = 4000
	if len(dialogText) > maxMessageLength {
		parts := splitMessageForManager(dialogText, maxMessageLength)
		for i, part := range parts {
			msg := tgbotapi.NewMessage(chatID, part)
			if i == len(parts)-1 {
				// Кнопка "Назад" только к последнему сообщению
				keyboard := tgbotapi.NewInlineKeyboardMarkup(
					tgbotapi.NewInlineKeyboardRow(
						tgbotapi.NewInlineKeyboardButtonData("🔙 К тикету", fmt.Sprintf("ticket_view_%d", ticketID)),
					),
				)
				msg.ReplyMarkup = keyboard
			}
			bot.Send(msg)
		}
	} else {
		msg := tgbotapi.NewMessage(chatID, dialogText)
		keyboard := tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("🔙 К тикету", fmt.Sprintf("ticket_view_%d", ticketID)),
			),
		)
		msg.ReplyMarkup = keyboard
		bot.Send(msg)
	}
}

// splitMessageForManager разбивает длинное сообщение на части для менеджера
func splitMessageForManager(text string, maxLength int) []string {
	var parts []string
	runes := []rune(text)

	for len(runes) > 0 {
		if len(runes) <= maxLength {
			parts = append(parts, string(runes))
			break
		}

		cutIndex := maxLength
		for cutIndex > maxLength/2 && runes[cutIndex] != '\n' {
			cutIndex--
		}

		if cutIndex <= maxLength/2 {
			cutIndex = maxLength
		}

		parts = append(parts, string(runes[:cutIndex]))
		runes = runes[cutIndex:]
	}

	return parts
}

func showTicketDetails(bot *tgbotapi.BotAPI, chatID int64, ticketID int) {
	ticket, exists := tickets[ticketID]
	if !exists {
		msg := tgbotapi.NewMessage(chatID, "❌ Тикет не найден")
		bot.Send(msg)
		return
	}

	oversizeText := "Нет"
	if ticket.Oversize {
		oversizeText = "Да"
	}

	status := "🟢 Открыт"
	if ticket.Status == "closed" {
		status = "🔴 Закрыт"
	}

	var text string
	if ticket.Height > 0 && ticket.ChestSize > 0 {
		// Есть данные подбора размера
		text = fmt.Sprintf("🎫 Тикет #%d %s\n\n"+
			"👤 Клиент: %s %s (@%s)\n"+
			"🆔 ID: %d\n"+
			"📏 Рост: %d см\n"+
			"📐 Обхват груди: %d см\n"+
			"👕 Оверсайз: %s\n"+
			"✅ Рекомендуемый размер: %s\n"+
			"🕐 Создан: %s\n",
			ticket.ID, status,
			ticket.FirstName, ticket.LastName, ticket.Username,
			ticket.UserID,
			ticket.Height,
			ticket.ChestSize,
			oversizeText,
			ticket.RecommendedSize,
			ticket.CreatedAt.Format("15:04 02.01.2006"))
	} else {
		// Нет данных подбора размера
		text = fmt.Sprintf("🎫 Тикет #%d %s\n\n"+
			"👤 Клиент: %s %s (@%s)\n"+
			"🆔 ID: %d\n"+
			"📏 Рост: Не указан\n"+
			"📐 Обхват груди: Не указан\n"+
			"👕 Оверсайз: Не указан\n"+
			"✅ Рекомендуемый размер: %s\n"+
			"🕐 Создан: %s\n",
			ticket.ID, status,
			ticket.FirstName, ticket.LastName, ticket.Username,
			ticket.UserID,
			ticket.RecommendedSize,
			ticket.CreatedAt.Format("15:04 02.01.2006"))
	}

	// Добавляем последние сообщения
	if len(ticket.Messages) > 0 {
		text += "\n💬 Последние сообщения:\n\n"
		lastMessages := getLastMessages(ticketID, 5)
		for _, msg := range lastMessages {
			senderType := "👤 Клиент"
			if msg.IsFromManager {
				senderType = "👨‍💼 Менеджер"
			}
			text += fmt.Sprintf("%s (%s):\n%s\n\n",
				senderType,
				msg.Time.Format("02.01 15:04"),
				msg.Text)
		}
	} else {
		text += "\n💬 Сообщений пока нет"
	}

	msg := tgbotapi.NewMessage(chatID, text)

	// Создаем кнопки действий
	var keyboard [][]tgbotapi.InlineKeyboardButton

	// Добавляем кнопку просмотра диалога
	if len(ticket.Messages) > 0 {
		keyboard = append(keyboard, []tgbotapi.InlineKeyboardButton{
			tgbotapi.NewInlineKeyboardButtonData(fmt.Sprintf("📋 Диалог (%d сообщений)", len(ticket.Messages)), fmt.Sprintf("ticket_dialog_%d", ticketID)),
		})
	}

	if ticket.Status == "open" {
		// Для открытых тикетов: ответить и закрыть
		keyboard = append(keyboard, []tgbotapi.InlineKeyboardButton{
			tgbotapi.NewInlineKeyboardButtonData("💬 Ответить", fmt.Sprintf("ticket_reply_%d", ticketID)),
			tgbotapi.NewInlineKeyboardButtonData("🔒 Закрыть", fmt.Sprintf("ticket_close_%d", ticketID)),
		})
	} else {
		// Для закрытых тикетов: открыть
		keyboard = append(keyboard, []tgbotapi.InlineKeyboardButton{
			tgbotapi.NewInlineKeyboardButtonData("🔓 Открыть", fmt.Sprintf("ticket_open_%d", ticketID)),
		})
	}

	// Кнопка "Назад"
	keyboard = append(keyboard, []tgbotapi.InlineKeyboardButton{
		tgbotapi.NewInlineKeyboardButtonData("🔙 Назад к списку", "manager_tickets"),
	})

	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(keyboard...)
	bot.Send(msg)
}

func startTicketReply(bot *tgbotapi.BotAPI, chatID int64, ticketID int) {
	// Сохраняем ID тикета для ответа
	userTickets[chatID] = ticketID

	msg := tgbotapi.NewMessage(chatID, fmt.Sprintf("💬 Ответ в тикет #%d\n\nНапишите ваш ответ клиенту:", ticketID))

	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("❌ Отмена", fmt.Sprintf("ticket_view_%d", ticketID)),
		),
	)

	msg.ReplyMarkup = keyboard
	bot.Send(msg)
}

func closeTicketFromButton(bot *tgbotapi.BotAPI, chatID int64, ticketID int) {
	ticket, exists := tickets[ticketID]
	if !exists {
		msg := tgbotapi.NewMessage(chatID, "❌ Тикет не найден")
		bot.Send(msg)
		return
	}

	if ticket.Status == "closed" {
		msg := tgbotapi.NewMessage(chatID, "❌ Тикет уже закрыт")
		bot.Send(msg)
		return
	}

	// Закрываем тикет
	ticket.Status = "closed"

	// Сохраняем изменения в файл
	saveTickets()

	// Уведомляем клиента
	closeMsg := tgbotapi.NewMessage(ticket.UserID, "🔒 Диалог с менеджером завершен.\n\nСпасибо за обращение! Если у вас есть другие вопросы, создайте новый диалог.")
	bot.Send(closeMsg)

	// Удаляем состояние вопроса
	delete(questionStates, ticket.UserID)

	// Подтверждаем менеджеру
	confirmMsg := tgbotapi.NewMessage(chatID, fmt.Sprintf("✅ Тикет #%d закрыт", ticketID))
	bot.Send(confirmMsg)

	log.Printf("Тикет #%d закрыт менеджером через кнопку", ticketID)
}

func openTicketFromButton(bot *tgbotapi.BotAPI, chatID int64, ticketID int) {
	ticket, exists := tickets[ticketID]
	if !exists {
		msg := tgbotapi.NewMessage(chatID, "❌ Тикет не найден")
		bot.Send(msg)
		return
	}

	if ticket.Status == "open" {
		msg := tgbotapi.NewMessage(chatID, "❌ Тикет уже открыт")
		bot.Send(msg)
		return
	}

	// Открываем тикет
	ticket.Status = "open"

	// Сохраняем изменения в файл
	saveTickets()

	// Уведомляем клиента
	openMsg := tgbotapi.NewMessage(ticket.UserID, "🔓 Диалог с менеджером возобновлен.\n\nВы можете продолжить общение в этом чате.")
	bot.Send(openMsg)

	// Включаем режим диалога для клиента
	questionStates[ticket.UserID] = true

	// Подтверждаем менеджеру
	confirmMsg := tgbotapi.NewMessage(chatID, fmt.Sprintf("✅ Тикет #%d открыт", ticketID))
	bot.Send(confirmMsg)

	log.Printf("Тикет #%d открыт менеджером через кнопку", ticketID)
}

func showTicketsWithButtons(bot *tgbotapi.BotAPI, chatID int64, ticketsToShow map[int]*Ticket, title string) {
	var text strings.Builder
	text.WriteString(fmt.Sprintf("%s (%d):\n\n", title, len(ticketsToShow)))

	// Сортируем тикеты по ID
	var ticketIDs []int
	for id := range ticketsToShow {
		ticketIDs = append(ticketIDs, id)
	}
	sort.Ints(ticketIDs)

	// Показываем первые 10 тикетов
	limit := 10
	if len(ticketIDs) > limit {
		limit = len(ticketIDs)
	}

	for i := 0; i < limit && i < len(ticketIDs); i++ {
		ticket := ticketsToShow[ticketIDs[i]]
		status := "🟢"
		if ticket.Status == "closed" {
			status = "🔴"
		}

		text.WriteString(fmt.Sprintf("%s #%d %s %s\n",
			status,
			ticket.ID,
			ticket.FirstName,
			ticket.LastName))
	}

	if len(ticketIDs) > 10 {
		text.WriteString(fmt.Sprintf("\n... и еще %d тикетов", len(ticketIDs)-10))
	}

	msg := tgbotapi.NewMessage(chatID, text.String())

	// Создаем кнопки для тикетов (максимум 5 в ряд)
	var keyboard [][]tgbotapi.InlineKeyboardButton
	for i := 0; i < limit && i < len(ticketIDs); i++ {
		ticketID := ticketIDs[i]
		ticket := ticketsToShow[ticketID]

		buttonText := fmt.Sprintf("#%d %s", ticketID, ticket.FirstName)
		if len(buttonText) > 20 {
			buttonText = fmt.Sprintf("#%d", ticketID)
		}

		button := tgbotapi.NewInlineKeyboardButtonData(buttonText, fmt.Sprintf("ticket_view_%d", ticketID))

		// Добавляем кнопку в ряд
		if len(keyboard) == 0 || len(keyboard[len(keyboard)-1]) >= 2 {
			keyboard = append(keyboard, []tgbotapi.InlineKeyboardButton{button})
		} else {
			keyboard[len(keyboard)-1] = append(keyboard[len(keyboard)-1], button)
		}
	}

	// Добавляем кнопку "Назад"
	keyboard = append(keyboard, []tgbotapi.InlineKeyboardButton{
		tgbotapi.NewInlineKeyboardButtonData("🔙 Назад", "back_to_manager_menu"),
	})

	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(keyboard...)
	bot.Send(msg)
}

func handleTicketButtonCallback(bot *tgbotapi.BotAPI, chatID int64, callbackData string) {
	// Обрабатываем кнопки тикетов
	if strings.HasPrefix(callbackData, "ticket_view_") {
		ticketIDStr := strings.TrimPrefix(callbackData, "ticket_view_")
		ticketID, err := strconv.Atoi(ticketIDStr)
		if err != nil {
			msg := tgbotapi.NewMessage(chatID, "❌ Ошибка ID тикета")
			bot.Send(msg)
			return
		}
		showTicketDetails(bot, chatID, ticketID)
	} else if strings.HasPrefix(callbackData, "ticket_reply_") {
		ticketIDStr := strings.TrimPrefix(callbackData, "ticket_reply_")
		ticketID, err := strconv.Atoi(ticketIDStr)
		if err != nil {
			msg := tgbotapi.NewMessage(chatID, "❌ Ошибка ID тикета")
			bot.Send(msg)
			return
		}
		startTicketReply(bot, chatID, ticketID)
	} else if strings.HasPrefix(callbackData, "ticket_close_") {
		ticketIDStr := strings.TrimPrefix(callbackData, "ticket_close_")
		ticketID, err := strconv.Atoi(ticketIDStr)
		if err != nil {
			msg := tgbotapi.NewMessage(chatID, "❌ Ошибка ID тикета")
			bot.Send(msg)
			return
		}
		closeTicketFromButton(bot, chatID, ticketID)
	} else if strings.HasPrefix(callbackData, "ticket_open_") {
		ticketIDStr := strings.TrimPrefix(callbackData, "ticket_open_")
		ticketID, err := strconv.Atoi(ticketIDStr)
		if err != nil {
			msg := tgbotapi.NewMessage(chatID, "❌ Ошибка ID тикета")
			bot.Send(msg)
			return
		}
		openTicketFromButton(bot, chatID, ticketID)
	} else if strings.HasPrefix(callbackData, "ticket_dialog_") {
		ticketIDStr := strings.TrimPrefix(callbackData, "ticket_dialog_")
		ticketID, err := strconv.Atoi(ticketIDStr)
		if err != nil {
			msg := tgbotapi.NewMessage(chatID, "❌ Ошибка ID тикета")
			bot.Send(msg)
			return
		}
		showManagerTicketDialog(bot, chatID, ticketID)
	}
}


func showContactManagerMenu(bot *tgbotapi.BotAPI, chatID int64) {
	msg := tgbotapi.NewMessage(chatID, "Выберите способ связи с менеджером:")

	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Написать менеджеру", "contact_manager_direct"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Назад в меню", "back_to_menu"),
		),
	)

	msg.ReplyMarkup = keyboard
	bot.Send(msg)
}

func contactManagerDirect(bot *tgbotapi.BotAPI, chatID int64) {
	// Проверяем, есть ли у пользователя активный тикет
	if ticketID, exists := userTickets[chatID]; exists {
		if ticket, found := tickets[ticketID]; found && ticket.Status == "open" {
			msg := tgbotapi.NewMessage(chatID, "💬 У вас уже есть активный диалог с менеджером!\n\nВы можете продолжить общение в этом чате. Менеджер получит ваши сообщения.")
			bot.Send(msg)
			return
		}
	}

	// Создаем тикет сразу и просим написать вопрос
	createTicketAndAskQuestion(bot, chatID, "Не определен")
}

func handleManagerReplyToTicket(bot *tgbotapi.BotAPI, message *tgbotapi.Message, ticketID int) {
	ticket, exists := tickets[ticketID]
	if !exists {
		msg := tgbotapi.NewMessage(message.Chat.ID, "❌ Тикет не найден")
		bot.Send(msg)
		delete(userTickets, message.Chat.ID)
		return
	}

	if ticket.Status != "open" {
		msg := tgbotapi.NewMessage(message.Chat.ID, "❌ Тикет закрыт")
		bot.Send(msg)
		delete(userTickets, message.Chat.ID)
		return
	}

	replyText := message.Text

	// Добавляем сообщение менеджера в тикет
	addMessageToTicket(ticketID, message.Chat.ID, replyText, true)

	// Отправляем ответ клиенту
	responseMsg := tgbotapi.NewMessage(ticket.UserID, fmt.Sprintf("💬 Ответ от менеджера:\n\n%s", replyText))
	bot.Send(responseMsg)

	// Удаляем состояние ответа
	delete(userTickets, message.Chat.ID)

	// Подтверждаем менеджеру
	confirmMsg := tgbotapi.NewMessage(message.Chat.ID, fmt.Sprintf("✅ Ответ отправлен в тикет #%d", ticketID))
	bot.Send(confirmMsg)

	log.Printf("Менеджер ответил в тикет #%d через кнопку: %s", ticketID, replyText)
}
