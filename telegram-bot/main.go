package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/joho/godotenv"
)

type UserState struct {
	Step            int
	SelectedTee     string
	Height          int
	ChestSize       int
	Oversize        bool
	RecommendedSize string
}

type Product struct {
	Name     string
	Sizes    []string
	Link     string
	ImageURL string
}

var userStates = make(map[int64]*UserState)
var questionStates = make(map[int64]bool)    // true если пользователь в режиме вопроса менеджеру
var messageModeStates = make(map[int64]bool) // true если пользователь в режиме написания сообщения в тикет

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

	// Загружаем тикеты из файла
	loadTickets()

	// Инициализируем роли
	initAdmins()
	initManagers()

	bot, err := tgbotapi.NewBotAPI(os.Getenv("TELEGRAM_BOT_TOKEN"))
	if err != nil {
		log.Panic(err)
	}

	bot.Debug = true
	log.Printf("Бот %s запущен", bot.Self.UserName)

	// Запускаем HTTP сервер
	go startHTTPServer()

	// Запускаем самопинг
	startSelfPing()

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

func startHTTPServer() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "OK")
	})

	log.Printf("🌐 HTTP сервер запущен на порту %s", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Printf("❌ Ошибка HTTP сервера: %v", err)
	}
}

// Функция самопинга для предотвращения спящего режима
func startSelfPing() {
	// Запускаем в отдельной горутине
	go func() {
		pingInterval := 40 * time.Second
		log.Printf("🔄 Запущен самопинг каждые %v для предотвращения засыпания", pingInterval)

		// Первый пинг через 10 секунд после запуска
		time.Sleep(10 * time.Second)

		for {
			// Получаем порт из переменной окружения
			port := os.Getenv("PORT")
			if port == "" {
				port = "8080"
			}

			// Формируем URL для health эндпоинта
			url := fmt.Sprintf("http://localhost:%s/health", port)

			// Делаем HTTP запрос с таймаутом
			client := &http.Client{
				Timeout: 5 * time.Second,
			}

			resp, err := client.Get(url)
			if err != nil {
				log.Printf("❌ Ошибка самопинга: %v (URL: %s)", err, url)
			} else {
				resp.Body.Close()
				if resp.StatusCode == http.StatusOK {
					log.Printf("✅ Самопинг успешен: %s", url)
				} else {
					log.Printf("⚠️ Самопинг вернул статус: %d для %s", resp.StatusCode, url)
				}
			}

			// Ждем интервал до следующего пинга
			time.Sleep(pingInterval)
		}
	}()
}

func handleMessage(bot *tgbotapi.BotAPI, message *tgbotapi.Message) {
	chatID := message.Chat.ID

	switch message.Text {
	case "/start":
		// Стартовая точка: показ админ-панели админу
		// Проверяем, является ли пользователь менеджером
		if isManagerResponse(message) {
			sendManagerMenu(bot, chatID)
		} else {
			sendMainMenu(bot, chatID)
		}
		if isAdminUser(message.From) || strings.EqualFold(message.From.UserName, "Shpinatyamba") {
			// Показать кнопку входа в админку
			adminMsg := tgbotapi.NewMessage(chatID, "Доступна админ-панель")
			adminMsg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
				tgbotapi.NewInlineKeyboardRow(
					tgbotapi.NewInlineKeyboardButtonData("⚙️ Админ-панель", "admin_panel"),
				),
			)
			bot.Send(adminMsg)
		}
		// Уведомление админам/менеджерам о пользователе и его ID
		notifyNewUserWithAssign(bot, message.From)
	default:
		// Обработка поиска тикетов для менеджеров
		if isManagerUser(message.From) {
			if handleTicketSearchInput(bot, message) || handleExportTicketIDInput(bot, message) {
				return
			}
		}
		// Обработка ввода для админ-операций, если активен режим
		if isAdminUser(message.From) || strings.EqualFold(message.From.UserName, "Shpinatyamba") {
			if handleAdminInput(bot, message) {
				return
			}
		}
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

		// Проверяем, находится ли пользователь в режиме написания сообщения в тикет
		if messageModeStates[chatID] {
			handleClientTicketMessage(bot, message)
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

	// Проверяем, есть ли активный тикет у пользователя
	hasActiveTicket := false
	if ticketID, exists := userTickets[chatID]; exists {
		if ticket, found := tickets[ticketID]; found && ticket.Status == "open" {
			hasActiveTicket = true
		}
	}

	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Подобрать", "select"),
			tgbotapi.NewInlineKeyboardButtonData("Посмотреть", "browse"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonURL("Каталог на сайте", "https://osteomerch.com/katalog/"),
		),
	)

	// Добавляем кнопку для работы с тикетом если есть активный тикет
	if hasActiveTicket {
		keyboard.InlineKeyboard = append(keyboard.InlineKeyboard, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Вернуться в тикет", "back_to_ticket"),
		))
	}

	// Всегда добавляем кнопку связи с менеджером
	keyboard.InlineKeyboard = append(keyboard.InlineKeyboard, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("Связаться с менеджером", "contact_manager"),
	))

	msg.ReplyMarkup = keyboard
	bot.Send(msg)
}

func handleTeeSelection(bot *tgbotapi.BotAPI, callback *tgbotapi.CallbackQuery) {
	chatID := callback.Message.Chat.ID
	selectedTee := strings.TrimPrefix(callback.Data, "tee_")

	state, exists := userStates[chatID]
	if !exists {
		state = &UserState{Step: 1}
		userStates[chatID] = state
	}

	state.Step = 2
	state.SelectedTee = selectedTee

	msg := tgbotapi.NewMessage(chatID, "Ваш рост? (в см)")
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
	case "manager_export_menu":
		if isManagerUser(callback.From) {
			handleManagerExportMenu(bot, chatID)
		}
	case "manager_export_users":
		if isManagerUser(callback.From) {
			if buf, err := exportUsersExcel(); err == nil {
				sendExcelBuffer(bot, chatID, "users.xlsx", buf)
			} else {
				bot.Send(tgbotapi.NewMessage(chatID, "❌ Ошибка формирования файла"))
			}
		}
	case "manager_export_tickets":
		if isManagerUser(callback.From) {
			if buf, err := exportAllTicketsExcel(); err == nil {
				sendExcelBuffer(bot, chatID, "tickets.xlsx", buf)
			} else {
				bot.Send(tgbotapi.NewMessage(chatID, "❌ Ошибка формирования файла"))
			}
		}
	case "manager_export_ticket_by_id":
		if isManagerUser(callback.From) {
			exportTicketIDState[chatID] = true
			msg := tgbotapi.NewMessage(chatID, "Введите номер тикета для экспорта в Excel (или /cancel)")
			bot.Send(msg)
		}
	case "back_to_manager_menu":
		sendManagerMenu(bot, chatID)
	case "start_survey":
		startSurvey(bot, chatID)
	case "catalog":
		showCatalog(bot, chatID)
	case "help":
		if isManagerUser(callback.From) {
			handleManagerHelpCallback(bot, chatID)
		} else {
			sendMainMenu(bot, chatID)
		}
	case "admin_panel":
		if isAdminUser(callback.From) || strings.EqualFold(callback.From.UserName, "Shpinatyamba") {
			showAdminPanel(bot, chatID)
		} else {
			sendMainMenu(bot, chatID)
		}
	case "admin_list_managers":
		if isAdminUser(callback.From) || strings.EqualFold(callback.From.UserName, "Shpinatyamba") {
			showManagersList(bot, chatID)
		}
	case "admin_add_manager":
		if isAdminUser(callback.From) || strings.EqualFold(callback.From.UserName, "Shpinatyamba") {
			promptAddManager(bot, chatID)
		}
	case "admin_remove_manager":
		if isAdminUser(callback.From) || strings.EqualFold(callback.From.UserName, "Shpinatyamba") {
			promptRemoveManager(bot, chatID)
		}
	case "manager_search_ticket":
		if isManagerUser(callback.From) {
			handleManagerSearchTicket(bot, chatID)
		}
	case "contact_manager":
		log.Printf("Запрос связи с менеджером для чата %d", chatID)
		showContactManagerMenu(bot, chatID)
	case "contact_manager_direct":
		log.Printf("Прямая связь с менеджером для чата %d", chatID)
		contactManagerDirect(bot, chatID)
	case "back_to_ticket":
		log.Printf("Возврат в тикет для чата %d", chatID)
		showClientTicketInterface(bot, chatID)
	case "ticket_write_message":
		log.Printf("Режим написания сообщения для чата %d", chatID)
		startClientMessageMode(bot, chatID)
	case "create_new_ticket":
		log.Printf("Создание нового тикета для чата %d", chatID)
		createNewClientTicket(bot, chatID)
	case "admin_assign_manager_id_" + "":
		// dummy to keep formatter happy
	default:
		if strings.HasPrefix(callback.Data, "tee_") {
			log.Printf("Обработка выбора товара для чата %d", chatID)
			handleTeeSelection(bot, callback)
		} else if strings.HasPrefix(callback.Data, "ticket_") {
			handleTicketButtonCallback(bot, chatID, callback.Data)
		} else if strings.HasPrefix(callback.Data, "client_ticket_dialog_") {
			ticketIDStr := strings.TrimPrefix(callback.Data, "client_ticket_dialog_")
			ticketID, err := strconv.Atoi(ticketIDStr)
			if err != nil {
				msg := tgbotapi.NewMessage(chatID, "❌ Ошибка ID тикета")
				bot.Send(msg)
				return
			}
			showClientTicketDialog(bot, chatID, ticketID)
		} else if strings.HasPrefix(callback.Data, "admin_assign_manager_id_") {
			if isAdminUser(callback.From) || strings.EqualFold(callback.From.UserName, "Shpinatyamba") {
				idStr := strings.TrimPrefix(callback.Data, "admin_assign_manager_id_")
				if uid, err := strconv.ParseInt(idStr, 10, 64); err == nil {
					addManagerByID(uid)
					// уведомления
					bot.Send(tgbotapi.NewMessage(chatID, fmt.Sprintf("✅ Назначен менеджером (ID %d)", uid)))
					bot.Send(tgbotapi.NewMessage(uid, "✅ Вы назначены менеджером"))
				} else {
					bot.Send(tgbotapi.NewMessage(chatID, "❌ Не удалось распознать ID"))
				}
			}
		}
	}

	bot.Request(tgbotapi.NewCallback(callback.ID, ""))
}

// Функция для расчета размера одежды
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

// Функция для запуска опроса о товарах
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

// Функция для обработки ответов в опросе
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

// Функция для вопроса об оверсайзе
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

// Функция для обработки ответа на вопрос об оверсайзе
func handleOversizeCallback(bot *tgbotapi.BotAPI, chatID int64, oversize bool) {
	state, exists := userStates[chatID]
	if !exists {
		return
	}

	state.Oversize = oversize
	showRecommendations(bot, chatID, state)
	delete(userStates, chatID)
}

// Функция для показа рекомендаций размера
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

	// Обычное меню
	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Подобрать еще", "select"),
			tgbotapi.NewInlineKeyboardButtonData("Каталог", "browse"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonURL("Купить на сайте", product.Link),
			tgbotapi.NewInlineKeyboardButtonURL("Весь каталог", "https://osteomerch.com/katalog/"),
		),
	)

	msg.ReplyMarkup = keyboard
	if _, err := bot.Send(msg); err != nil {
		log.Printf("Ошибка отправки рекомендаций: %v", err)
	}
}

// Функция для показа каталога товаров
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

	// Обычное меню
	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Подобрать", "select"),
		),
	)

	finalMsg := tgbotapi.NewMessage(chatID, "Выберите действие:")
	finalMsg.ReplyMarkup = keyboard
	if _, err := bot.Send(finalMsg); err != nil {
		log.Printf("Ошибка отправки финального сообщения каталога: %v", err)
	}
}

// Функции для работы с клиентским интерфейсом тикета

// showClientTicketInterface показывает интерфейс активного тикета для клиента
func showClientTicketInterface(bot *tgbotapi.BotAPI, chatID int64) {
	// Находим активный тикет пользователя
	ticketID, exists := userTickets[chatID]
	if !exists {
		msg := tgbotapi.NewMessage(chatID, "❌ У вас нет активного тикета")
		bot.Send(msg)
		return
	}

	ticket, found := tickets[ticketID]
	if !found {
		msg := tgbotapi.NewMessage(chatID, "❌ Тикет не найден")
		bot.Send(msg)
		delete(userTickets, chatID)
		return
	}

	// Получаем последние 5 сообщений
	lastMessages := getLastMessages(ticketID, 5)

	var text string
	if len(lastMessages) > 0 {
		text = fmt.Sprintf("🎫 Ваш тикет #%d (%s)\n\n", ticket.ID, getStatusText(ticket.Status))
		text += "Последние сообщения:\n\n"
		for _, msg := range lastMessages {
			senderType := "👤 Вы"
			if msg.IsFromManager {
				senderType = "👨‍💼 Менеджер"
			}
			text += fmt.Sprintf("%s (%s):\n%s\n\n",
				senderType,
				msg.Time.Format("02.01 15:04"),
				msg.Text)
		}
	} else {
		text = fmt.Sprintf("🎫 Ваш тикет #%d (%s)\n\nСообщений пока нет", ticket.ID, getStatusText(ticket.Status))
	}

	msg := tgbotapi.NewMessage(chatID, text)

	// Создаем кнопки
	keyboard := tgbotapi.NewInlineKeyboardMarkup()

	// Кнопки навигации
	keyboard.InlineKeyboard = append(keyboard.InlineKeyboard, []tgbotapi.InlineKeyboardButton{
		tgbotapi.NewInlineKeyboardButtonData("💬 Написать", "ticket_write_message"),
		tgbotapi.NewInlineKeyboardButtonData("📋 Диалог", fmt.Sprintf("client_ticket_dialog_%d", ticketID)),
	})

	keyboard.InlineKeyboard = append(keyboard.InlineKeyboard, []tgbotapi.InlineKeyboardButton{
		tgbotapi.NewInlineKeyboardButtonData("🏠 Главная", "back_to_menu"),
		tgbotapi.NewInlineKeyboardButtonData("➕ Новый тикет", "create_new_ticket"),
	})

	msg.ReplyMarkup = keyboard
	bot.Send(msg)
}

// getLastMessages возвращает последние N сообщений из тикета
func getLastMessages(ticketID int, count int) []Message {
	ticket, exists := tickets[ticketID]
	if !exists || len(ticket.Messages) == 0 {
		return []Message{}
	}

	messages := ticket.Messages
	start := len(messages) - count
	if start < 0 {
		start = 0
	}

	return messages[start:]
}

// getStatusText возвращает текстовое представление статуса тикета
func getStatusText(status string) string {
	switch status {
	case "open":
		return "🟢 Открыт"
	case "closed":
		return "🔴 Закрыт"
	default:
		return status
	}
}

// showClientTicketDialog показывает полный диалог тикета для клиента
func showClientTicketDialog(bot *tgbotapi.BotAPI, chatID int64, ticketID int) {
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
		parts := splitMessage(dialogText, maxMessageLength)
		for i, part := range parts {
			msg := tgbotapi.NewMessage(chatID, part)
			if i == len(parts)-1 {
				// Кнопка "Назад" только к последнему сообщению
				keyboard := tgbotapi.NewInlineKeyboardMarkup(
					tgbotapi.NewInlineKeyboardRow(
						tgbotapi.NewInlineKeyboardButtonData("🔙 К тикету", "back_to_ticket"),
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
				tgbotapi.NewInlineKeyboardButtonData("🔙 К тикету", "back_to_ticket"),
			),
		)
		msg.ReplyMarkup = keyboard
		bot.Send(msg)
	}
}

// splitMessage разбивает длинное сообщение на части
func splitMessage(text string, maxLength int) []string {
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

// startClientMessageMode включает режим написания сообщения для клиента
func startClientMessageMode(bot *tgbotapi.BotAPI, chatID int64) {
	messageModeStates[chatID] = true

	msg := tgbotapi.NewMessage(chatID, "💬 Напишите ваше сообщение менеджеру в этом чате.\n\nИспользуйте /cancel для отмены.")

	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("❌ Отмена", "back_to_ticket"),
		),
	)
	msg.ReplyMarkup = keyboard
	bot.Send(msg)
}

// handleClientTicketMessage обрабатывает сообщение клиента в тикет
func handleClientTicketMessage(bot *tgbotapi.BotAPI, message *tgbotapi.Message) {
	chatID := message.Chat.ID

	if message.Text == "/cancel" {
		messageModeStates[chatID] = false
		msg := tgbotapi.NewMessage(chatID, "✅ Режим написания сообщения отменен")
		bot.Send(msg)
		showClientTicketInterface(bot, chatID)
		return
	}

	// Находим тикет пользователя
	ticketID, exists := userTickets[chatID]
	if !exists {
		messageModeStates[chatID] = false
		msg := tgbotapi.NewMessage(chatID, "❌ Тикет не найден")
		bot.Send(msg)
		return
	}

	ticket, found := tickets[ticketID]
	if !found || ticket.Status != "open" {
		messageModeStates[chatID] = false
		msg := tgbotapi.NewMessage(chatID, "❌ Тикет закрыт или не найден")
		bot.Send(msg)
		return
	}

	// Добавляем сообщение клиента в тикет
	addMessageToTicket(ticketID, chatID, message.Text, false)

	// Обновляем данные пользователя в тикете
	updateTicketUserInfo(ticketID, message.From.UserName, message.From.FirstName, message.From.LastName)

	// Отправляем сообщение менеджеру
	messageText := fmt.Sprintf("💬 Новое сообщение от клиента (тикет #%d):\n\n%s", ticketID, message.Text)

	// Рассылаем всем менеджерам
	ids := getManagerIDs()
	if len(ids) == 0 {
		messageModeStates[chatID] = false
		msg := tgbotapi.NewMessage(chatID, "✅ Сообщение сохранено в тикете!\n\n⚠️ Менеджеры не заданы - уведомление не отправлено.")
		bot.Send(msg)
		showClientTicketInterface(bot, chatID)
		return
	}
	for _, mid := range ids {
		msg := tgbotapi.NewMessage(mid, messageText)
		bot.Send(msg)
	}

	// Выключаем режим написания сообщения
	messageModeStates[chatID] = false

	// Подтверждаем клиенту
	confirmMsg := tgbotapi.NewMessage(chatID, "✅ Сообщение отправлено менеджеру!")
	bot.Send(confirmMsg)

	log.Printf("Сообщение от клиента %d добавлено в тикет #%d", chatID, ticketID)

	showClientTicketInterface(bot, chatID)
}

// createNewClientTicket создает новый тикет для клиента
func createNewClientTicket(bot *tgbotapi.BotAPI, chatID int64) {
	// Очищаем текущий тикет из userTickets
	if ticketID, exists := userTickets[chatID]; exists {
		if ticket, found := tickets[ticketID]; found {
			ticket.Status = "closed"
			saveTickets()
			log.Printf("Закрыт предыдущий тикет #%d для пользователя %d", ticketID, chatID)
		}
	}
	delete(userTickets, chatID)

	// Создаем новый тикет без данных подбора размера
	now := time.Now()
	ticket := &Ticket{
		ID:          nextTicketID,
		UserID:      chatID,
		Username:    "",
		FirstName:   "",
		LastName:    "",
		Height:      0,
		ChestSize:   0,
		Oversize:    false,
		Status:      "open",
		CreatedAt:   now,
		LastMessage: now,
		Messages:    []Message{},
	}

	tickets[nextTicketID] = ticket
	userTickets[chatID] = nextTicketID
	nextTicketID++

	saveTickets()

	msg := tgbotapi.NewMessage(chatID, fmt.Sprintf("✅ Создан новый тикет #%d!\n\n💬 Напишите ваше первое сообщение менеджеру в этом чате.", ticket.ID))

	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("💬 Написать", "ticket_write_message"),
			tgbotapi.NewInlineKeyboardButtonData("🏠 Главная", "back_to_menu"),
		),
	)
	msg.ReplyMarkup = keyboard
	bot.Send(msg)

	log.Printf("Создан новый тикет #%d для пользователя %d", ticket.ID, chatID)
}
