package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/joho/godotenv"
)

type UserState struct {
	Step        int
	SelectedTee string
	Height      int
	ChestSize   int
	Oversize    bool
}

type Ticket struct {
	ID              int
	UserID          int64
	Username        string
	FirstName       string
	LastName        string
	Height          int
	ChestSize       int
	Oversize        bool
	RecommendedSize string
	Question        string
	Status          string // "open", "closed"
	CreatedAt       time.Time
	LastMessage     time.Time
}

type ManagerQuestion struct {
	UserID    int64
	Username  string
	FirstName string
	LastName  string
	Question  string
	Timestamp time.Time
}

type Product struct {
	Name     string
	Sizes    []string
	Link     string
	ImageURL string
}

var userStates = make(map[int64]*UserState)
var questionStates = make(map[int64]bool) // true если пользователь в режиме вопроса менеджеру
var tickets = make(map[int]*Ticket)       // все тикеты
var userTickets = make(map[int64]int)     // связь пользователь -> ID тикета
var nextTicketID = 1
var managerID int64 = 0 // @Shpinatyamba - будет установлен автоматически при первом сообщении

var products = []Product{
	{"Футболка Крылатые Фразы белая", []string{"S", "M", "L", "XL", "XXL"}, "https://osteomerch.com/katalog/item/colorful-jumper-with-horizontal-stripes/", "./katalog/Крылатые Фразы/1.jpg"},
	{"Футболка Black to Black черная", []string{"S", "M", "L", "XL", "XXL"}, "https://osteomerch.com/katalog/item/black-suede-pleated-skirt/", "./katalog/Black to Black/1.jpg"},
	{"Футболка Black to Black 2 черная", []string{"S", "M", "L", "XL", "XXL"}, "https://osteomerch.com/katalog/item/black-wide-suede-pants-with-white-stripes/", "./katalog/Black to Black 2/1.jpg"},
}

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Println("Файл .env не найден, используем переменные окружения")
	}

	bot, err := tgbotapi.NewBotAPI(os.Getenv("TELEGRAM_BOT_TOKEN"))
	if err != nil {
		log.Panic(err)
	}

	bot.Debug = true
	log.Printf("Бот %s запущен", bot.Self.UserName)

	// Инициализируем ID менеджера
	managerIDStr := os.Getenv("MANAGER_ID")
	if managerIDStr != "" {
		var err error
		managerID, err = strconv.ParseInt(managerIDStr, 10, 64)
		if err != nil {
			log.Printf("Ошибка парсинга MANAGER_ID: %v", err)
			managerID = 123456789 // используем значение по умолчанию
		} else {
			log.Printf("ID менеджера установлен: %d", managerID)
		}
	} else {
		log.Printf("MANAGER_ID не установлен, используем значение по умолчанию: %d", managerID)
	}

	// Запускаем само пинг для Render
	go startSelfPing()

	// Бесконечный цикл с восстановлением
	for {
		runBot(bot)
		log.Println("Бот остановился, перезапуск через 5 секунд...")
		time.Sleep(5 * time.Second)
	}
}

func runBot(bot *tgbotapi.BotAPI) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("Паника в боте: %v", r)
		}
	}()

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := bot.GetUpdatesChan(u)

	for update := range updates {
		if update.Message != nil {
			handleMessage(bot, update.Message)
		} else if update.CallbackQuery != nil {
			handleCallbackQuery(bot, update.CallbackQuery)
		}
	}
}

func handleMessage(bot *tgbotapi.BotAPI, message *tgbotapi.Message) {
	chatID := message.Chat.ID

	switch message.Text {
	case "/start":
		// Проверяем, является ли пользователь менеджером
		if isManagerResponse(message) {
			sendManagerMenu(bot, chatID)
		} else {
			sendMainMenu(bot, chatID)
		}
	default:
		// Проверяем, является ли это ответом менеджера
		if isManagerResponse(message) {
			handleManagerResponse(bot, message)
			return
		}

		// Проверяем, находится ли пользователь в режиме вопроса менеджеру
		if questionStates[chatID] {
			handleManagerQuestion(bot, message)
			return
		}

		state, exists := userStates[chatID]
		if exists {
			handleSurveyResponse(bot, message, state)
		} else {
			msg := tgbotapi.NewMessage(chatID, "Используйте /start для начала работы")
			bot.Send(msg)
		}
	}
}

func sendMainMenu(bot *tgbotapi.BotAPI, chatID int64) {
	msg := tgbotapi.NewMessage(chatID, "Здравствуйте! Я бот Osteomerch. Если вы хотите подобрать для себя подходящий вариант одежды воспользуйтесь кнопками:")

	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Подобрать", "select"),
			tgbotapi.NewInlineKeyboardButtonData("Посмотреть", "browse"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonURL("Каталог на сайте", "https://osteomerch.com/katalog/"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Связаться с менеджером", "contact_manager"),
		),
	)

	msg.ReplyMarkup = keyboard
	bot.Send(msg)
}

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

func handleCallbackQuery(bot *tgbotapi.BotAPI, callback *tgbotapi.CallbackQuery) {
	chatID := callback.Message.Chat.ID
	log.Printf("Получен callback: %s для чата %d", callback.Data, chatID)

	switch callback.Data {
	case "select":
		log.Printf("Запуск опроса для чата %d", chatID)
		startSurvey(bot, chatID)
	case "browse":
		log.Printf("Показ каталога для чата %d", chatID)
		showCatalog(bot, chatID)
	case "contact_manager":
		log.Printf("Запрос связи с менеджером для чата %d", chatID)
		showContactManagerMenu(bot, chatID)
	case "contact_manager_direct":
		log.Printf("Прямая связь с менеджером для чата %d", chatID)
		contactManagerDirect(bot, chatID)
	case "back_to_menu":
		log.Printf("Возврат в главное меню для чата %d", chatID)
		sendMainMenu(bot, chatID)
	case "oversize_yes":
		handleOversizeCallback(bot, chatID, true)
	case "oversize_no":
		handleOversizeCallback(bot, chatID, false)
	case "manager_tickets":
		handleManagerTicketsCallback(bot, chatID)
	case "manager_open_tickets":
		handleManagerOpenTicketsCallback(bot, chatID)
	case "manager_closed_tickets":
		handleManagerClosedTicketsCallback(bot, chatID)
	case "manager_stats":
		handleManagerStatsCallback(bot, chatID)
	case "manager_help":
		handleManagerHelpCallback(bot, chatID)
	case "back_to_manager_menu":
		sendManagerMenu(bot, chatID)
	case "start_survey":
		// Если это менеджер, показываем менеджерское меню после опроса
		if isManagerResponse(&tgbotapi.Message{Chat: &tgbotapi.Chat{ID: chatID}}) {
			startSurvey(bot, chatID)
		} else {
			startSurvey(bot, chatID)
		}
	case "catalog":
		showCatalog(bot, chatID)
	case "help":
		// Если это менеджер, показываем менеджерскую помощь
		if isManagerResponse(&tgbotapi.Message{Chat: &tgbotapi.Chat{ID: chatID}}) {
			handleManagerHelpCallback(bot, chatID)
		} else {
			sendMainMenu(bot, chatID)
		}
	default:
		if strings.HasPrefix(callback.Data, "tee_") {
			selectedTee := strings.TrimPrefix(callback.Data, "tee_")
			log.Printf("Выбрана футболка %s для чата %d", selectedTee, chatID)
			startHeightQuestion(bot, chatID, selectedTee)
		} else if strings.HasPrefix(callback.Data, "ticket_") {
			handleTicketButtonCallback(bot, chatID, callback.Data)
		}
	}

	bot.Request(tgbotapi.NewCallback(callback.ID, ""))
}

func startSurvey(bot *tgbotapi.BotAPI, chatID int64) {
	log.Printf("Начинаю опрос для чата %d", chatID)
	userStates[chatID] = &UserState{Step: 1}

	msg := tgbotapi.NewMessage(chatID, "Выберите интересующий мерч:")
	if _, err := bot.Send(msg); err != nil {
		log.Printf("Ошибка отправки сообщения: %v", err)
		return
	}

	for i, product := range products {
		keyboard := tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("Выбрать", fmt.Sprintf("tee_%d", i)),
			),
		)

		// Пытаемся отправить фото
		photo := tgbotapi.NewPhoto(chatID, tgbotapi.FilePath(product.ImageURL))
		photo.Caption = fmt.Sprintf("%s\n\nРазмеры: %s", product.Name, strings.Join(product.Sizes, ", "))
		photo.ReplyMarkup = keyboard

		if _, err := bot.Send(photo); err != nil {
			log.Printf("Ошибка отправки фото для %s: %v, отправляю текстовое сообщение", product.Name, err)

			// Если фото не отправилось, отправляем текстовое сообщение
			textMsg := tgbotapi.NewMessage(chatID, fmt.Sprintf("%s\n\nРазмеры: %s", product.Name, strings.Join(product.Sizes, ", ")))
			textMsg.ReplyMarkup = keyboard
			if _, textErr := bot.Send(textMsg); textErr != nil {
				log.Printf("Ошибка отправки текстового сообщения: %v", textErr)
			}
		}
	}
}

func startHeightQuestion(bot *tgbotapi.BotAPI, chatID int64, selectedTee string) {
	// Проверяем, существует ли состояние пользователя
	state, exists := userStates[chatID]
	if !exists {
		// Если состояния нет, создаем новое
		state = &UserState{Step: 1}
		userStates[chatID] = state
	}

	state.Step = 2
	state.SelectedTee = selectedTee

	msg := tgbotapi.NewMessage(chatID, "Ваш рост? (в см)")
	bot.Send(msg)
}

func handleSurveyResponse(bot *tgbotapi.BotAPI, message *tgbotapi.Message, state *UserState) {
	chatID := message.Chat.ID

	switch state.Step {
	case 2:
		height, err := strconv.Atoi(message.Text)
		if err != nil {
			msg := tgbotapi.NewMessage(chatID, "Пожалуйста, введите рост в сантиметрах (например: 175)")
			bot.Send(msg)
			return
		}

		if height < 100 || height > 250 {
			msg := tgbotapi.NewMessage(chatID, "Рост должен быть от 100 до 250 см. Попробуйте еще раз:")
			bot.Send(msg)
			return
		}

		state.Height = height
		state.Step = 3
		msg := tgbotapi.NewMessage(chatID, "Обхват груди? (в см)")
		bot.Send(msg)

	case 3:
		chestSize, err := strconv.Atoi(message.Text)
		if err != nil {
			msg := tgbotapi.NewMessage(chatID, "Пожалуйста, введите обхват груди в сантиметрах (например: 90)")
			bot.Send(msg)
			return
		}

		if chestSize < 70 || chestSize > 130 {
			msg := tgbotapi.NewMessage(chatID, "Обхват груди должен быть от 70 до 130 см. Попробуйте еще раз:")
			bot.Send(msg)
			return
		}

		state.ChestSize = chestSize
		state.Step = 4
		askOversizeQuestion(bot, chatID)

	case 4:
		response := strings.ToLower(message.Text)
		switch response {
		case "да", "yes":
			state.Oversize = true
		case "нет", "no":
			state.Oversize = false
		default:
			msg := tgbotapi.NewMessage(chatID, "Пожалуйста, ответьте 'да' или 'нет'")
			bot.Send(msg)
			return
		}
		showRecommendations(bot, chatID, state)
		delete(userStates, chatID)
	}
}

func askOversizeQuestion(bot *tgbotapi.BotAPI, chatID int64) {
	msg := tgbotapi.NewMessage(chatID, "Хотите ли вы оверсайз модель?")

	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Да", "oversize_yes"),
			tgbotapi.NewInlineKeyboardButtonData("Нет", "oversize_no"),
		),
	)

	msg.ReplyMarkup = keyboard
	bot.Send(msg)
}

func handleOversizeCallback(bot *tgbotapi.BotAPI, chatID int64, oversize bool) {
	state, exists := userStates[chatID]
	if !exists {
		return
	}

	state.Oversize = oversize
	showRecommendations(bot, chatID, state)
	delete(userStates, chatID)
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
	createTicketAndAskQuestion(bot, chatID)
}

func createTicketAndAskQuestion(bot *tgbotapi.BotAPI, chatID int64) {
	// Проверяем, есть ли данные пользователя для создания тикета
	state, exists := userStates[chatID]

	// Создаем тикет с данными клиента (если есть) или без них
	var ticket *Ticket
	if exists {
		// Есть данные подбора размера
		recommendedSize := calculateSize(state.ChestSize, state.Oversize)
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
			CreatedAt:       time.Now(),
			LastMessage:     time.Now(),
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
			RecommendedSize: "Не определен",
			Question:        "",
			Status:          "open",
			CreatedAt:       time.Now(),
			LastMessage:     time.Now(),
		}
	}

	// Сохраняем тикет
	tickets[nextTicketID] = ticket
	userTickets[chatID] = nextTicketID
	nextTicketID++

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
	ticket.Username = message.From.UserName
	ticket.FirstName = message.From.FirstName
	ticket.LastName = message.From.LastName
	ticket.Question = question
	ticket.LastMessage = time.Now()

	// Отправляем сообщение менеджеру
	messageText := fmt.Sprintf("💬 Сообщение от клиента (тикет #%d):\n\n%s", ticketID, question)
	msg := tgbotapi.NewMessage(managerID, messageText)
	bot.Send(msg)

	log.Printf("Отправлено сообщение от пользователя %d в тикет #%d", chatID, ticketID)
}

func isManagerResponse(message *tgbotapi.Message) bool {
	// Проверяем по ID менеджера
	if managerID != 0 && message.From.ID == managerID {
		return true
	}

	// Проверяем по username (для автоматического определения)
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

	// Отправляем ответ клиенту
	responseMsg := tgbotapi.NewMessage(ticket.UserID, fmt.Sprintf("💬 Ответ от менеджера:\n\n%s", replyText))
	bot.Send(responseMsg)

	// Обновляем время последнего сообщения
	ticket.LastMessage = time.Now()

	// Удаляем состояние ответа
	delete(userTickets, message.Chat.ID)

	// Подтверждаем менеджеру
	confirmMsg := tgbotapi.NewMessage(message.Chat.ID, fmt.Sprintf("✅ Ответ отправлен в тикет #%d", ticketID))
	bot.Send(confirmMsg)

	log.Printf("Менеджер ответил в тикет #%d через кнопку: %s", ticketID, replyText)
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
	}
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
			"❓ Вопрос: %s\n"+
			"🕐 Создан: %s\n"+
			"💬 Последнее сообщение: %s",
			ticket.ID, status,
			ticket.FirstName, ticket.LastName, ticket.Username,
			ticket.UserID,
			ticket.Height,
			ticket.ChestSize,
			oversizeText,
			ticket.RecommendedSize,
			ticket.Question,
			ticket.CreatedAt.Format("15:04 02.01.2006"),
			ticket.LastMessage.Format("15:04 02.01.2006"))
	} else {
		// Нет данных подбора размера
		text = fmt.Sprintf("🎫 Тикет #%d %s\n\n"+
			"👤 Клиент: %s %s (@%s)\n"+
			"🆔 ID: %d\n"+
			"📏 Рост: Не указан\n"+
			"📐 Обхват груди: Не указан\n"+
			"👕 Оверсайз: Не указан\n"+
			"✅ Рекомендуемый размер: %s\n"+
			"❓ Вопрос: %s\n"+
			"🕐 Создан: %s\n"+
			"💬 Последнее сообщение: %s",
			ticket.ID, status,
			ticket.FirstName, ticket.LastName, ticket.Username,
			ticket.UserID,
			ticket.RecommendedSize,
			ticket.Question,
			ticket.CreatedAt.Format("15:04 02.01.2006"),
			ticket.LastMessage.Format("15:04 02.01.2006"))
	}

	msg := tgbotapi.NewMessage(chatID, text)

	// Создаем кнопки действий
	var keyboard [][]tgbotapi.InlineKeyboardButton

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

func showRecommendations(bot *tgbotapi.BotAPI, chatID int64, state *UserState) {
	log.Printf("Показываю рекомендации для чата %d, товар: %s", chatID, state.SelectedTee)

	// Проверяем, что SelectedTee не пустой
	if state.SelectedTee == "" {
		log.Printf("Ошибка: SelectedTee пустой для чата %d", chatID)
		msg := tgbotapi.NewMessage(chatID, "Произошла ошибка. Попробуйте еще раз.")
		bot.Send(msg)
		return
	}

	teeIndex, err := strconv.Atoi(state.SelectedTee)
	if err != nil {
		log.Printf("Ошибка парсинга индекса товара: %v", err)
		msg := tgbotapi.NewMessage(chatID, "Произошла ошибка. Попробуйте еще раз.")
		bot.Send(msg)
		return
	}

	if teeIndex < 0 || teeIndex >= len(products) {
		log.Printf("Неверный индекс товара: %d, доступно товаров: %d", teeIndex, len(products))
		msg := tgbotapi.NewMessage(chatID, "Произошла ошибка. Попробуйте еще раз.")
		bot.Send(msg)
		return
	}

	product := products[teeIndex]

	size := calculateSize(state.ChestSize, state.Oversize)

	responseText := fmt.Sprintf("Вам подойдут следующие размеры модели:\n\n%s - размер %s",
		product.Name, size)

	msg := tgbotapi.NewMessage(chatID, responseText)

	// Определяем, является ли пользователь менеджером
	isManager := isManagerResponse(&tgbotapi.Message{Chat: &tgbotapi.Chat{ID: chatID}})
	
	var keyboard tgbotapi.InlineKeyboardMarkup
	if isManager {
		// Менеджерское меню
		keyboard = tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("📏 Подобрать еще", "start_survey"),
				tgbotapi.NewInlineKeyboardButtonData("📚 Каталог", "catalog"),
			),
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("👥 Клиенты", "manager_tickets"),
			),
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonURL("Купить на сайте", product.Link),
				tgbotapi.NewInlineKeyboardButtonURL("Весь каталог", "https://osteomerch.com/katalog/"),
			),
		)
	} else {
		// Обычное меню
		keyboard = tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("Подобрать еще", "select"),
				tgbotapi.NewInlineKeyboardButtonData("Каталог", "browse"),
			),
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonURL("Купить на сайте", product.Link),
				tgbotapi.NewInlineKeyboardButtonURL("Весь каталог", "https://osteomerch.com/katalog/"),
			),
		)
	}

	msg.ReplyMarkup = keyboard
	if _, err := bot.Send(msg); err != nil {
		log.Printf("Ошибка отправки рекомендаций: %v", err)
	}
}

func calculateSize(chestSize int, oversize bool) string {
	// Определяем размер по обхвату груди согласно таблице
	var sizeRange string

	if chestSize >= 70 && chestSize <= 89 {
		sizeRange = "XS-S"
	} else if chestSize >= 90 && chestSize <= 97 {
		sizeRange = "M-L"
	} else if chestSize >= 98 && chestSize <= 105 {
		sizeRange = "XL-2XL"
	} else if chestSize >= 106 && chestSize <= 113 {
		sizeRange = "3XL-4XL"
	} else if chestSize >= 114 && chestSize <= 121 {
		sizeRange = "5XL-6XL"
	} else if chestSize >= 122 && chestSize <= 130 {
		sizeRange = "7XL-8XL"
	} else if chestSize < 70 {
		return "XS-S (размер меньше минимального)"
	} else {
		return "7XL-8XL (размер больше максимального)"
	}

	// Если запрошен оверсайз, берем больший размер из диапазона
	if oversize {
		switch sizeRange {
		case "XS-S":
			return "M-L"
		case "M-L":
			return "XL-2XL"
		case "XL-2XL":
			return "3XL-4XL"
		case "3XL-4XL":
			return "5XL-6XL"
		case "5XL-6XL":
			return "7XL-8XL"
		case "7XL-8XL":
			return "7XL-8XL (максимальный размер)"
		}
	}

	return sizeRange
}

func showCatalog(bot *tgbotapi.BotAPI, chatID int64) {
	log.Printf("Показываю каталог для чата %d", chatID)

	msg := tgbotapi.NewMessage(chatID, "Каталог товаров:")
	if _, err := bot.Send(msg); err != nil {
		log.Printf("Ошибка отправки сообщения каталога: %v", err)
		return
	}

	for _, product := range products {
		// Пытаемся отправить фото
		photo := tgbotapi.NewPhoto(chatID, tgbotapi.FilePath(product.ImageURL))
		photo.Caption = fmt.Sprintf("%s\nРазмеры: %s\nСсылка на сайт: [%s](%s)",
			product.Name, strings.Join(product.Sizes, ", "), product.Name, product.Link)
		photo.ParseMode = "MarkdownV2"

		if _, err := bot.Send(photo); err != nil {
			log.Printf("Ошибка отправки фото каталога для %s: %v, отправляю текстовое сообщение", product.Name, err)

			// Если фото не отправилось, отправляем текстовое сообщение
			textMsg := tgbotapi.NewMessage(chatID, fmt.Sprintf("%s\nРазмеры: %s\nСсылка на сайт: [%s](%s)",
				product.Name, strings.Join(product.Sizes, ", "), product.Name, product.Link))
			textMsg.ParseMode = "MarkdownV2"
			if _, textErr := bot.Send(textMsg); textErr != nil {
				log.Printf("Ошибка отправки текстового сообщения каталога: %v", textErr)
			}
		}
	}

	// Определяем, является ли пользователь менеджером
	isManager := isManagerResponse(&tgbotapi.Message{Chat: &tgbotapi.Chat{ID: chatID}})
	
	var keyboard tgbotapi.InlineKeyboardMarkup
	if isManager {
		// Менеджерское меню
		keyboard = tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("📏 Подобрать размер", "start_survey"),
			),
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("👥 Клиенты", "manager_tickets"),
			),
		)
	} else {
		// Обычное меню
		keyboard = tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("Подобрать", "select"),
			),
		)
	}

	finalMsg := tgbotapi.NewMessage(chatID, "Выберите действие:")
	finalMsg.ReplyMarkup = keyboard
	if _, err := bot.Send(finalMsg); err != nil {
		log.Printf("Ошибка отправки финального сообщения каталога: %v", err)
	}
}

func startSelfPing() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// Создаем HTTP сервер для само пинга
	http.HandleFunc("/ping", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("pong"))
	})

	// Запускаем сервер в отдельной горутине
	go func() {
		log.Printf("Запуск HTTP сервера для само пинга на порту %s", port)
		if err := http.ListenAndServe(":"+port, nil); err != nil {
			log.Printf("Ошибка запуска HTTP сервера: %v", err)
		}
	}()

	// Пингуем себя каждые 40 секунд
	url := fmt.Sprintf("http://localhost:%s/ping", port)

	for {
		time.Sleep(40 * time.Second)

		resp, err := http.Get(url)
		if err != nil {
			log.Printf("Ошибка само пинга: %v", err)
			continue
		}
		resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			log.Println("Само пинг выполнен успешно")
		} else {
			log.Printf("Само пинг вернул статус: %d", resp.StatusCode)
		}
	}
}
