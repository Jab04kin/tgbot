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
    FirstName       string
    LastName        string
    DopName         string
    ForSelf         *bool
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
var nameCollectState = make(map[int64]bool)  // true если ожидаем имя клиента для контакта с менеджером

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

	// Корневой маршрут — полезен для внешних пингов/проверок
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "OK")
	})

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

		// ensureHealthPath добавляет "/health" если путь пуст или корневой
		ensureHealthPath := func(base string) string {
			b := strings.TrimSpace(base)
			if b == "" {
				return "/health"
			}
			// если уже указан конкретный путь (не "/"), оставляем как есть
			if strings.HasSuffix(b, "/health") || strings.Contains(b, "/health?") {
				return b
			}
			// если это чистый домен/корень — добавим /health
			if strings.HasSuffix(b, "/") {
				return b + "health"
			}
			// если пути нет (например, https://app.onrender.com)
			if !strings.Contains(strings.TrimPrefix(b, "http://"), "/") && !strings.Contains(strings.TrimPrefix(b, "https://"), "/") {
				return b + "/health"
			}
			return b
		}

		// helper: получить целевой URL (приоритет: SELF_PING_URL -> RENDER_EXTERNAL_URL -> RENDER_URL -> localhost)
		resolveURL := func() string {
			if u := strings.TrimSpace(os.Getenv("SELF_PING_URL")); u != "" {
				return ensureHealthPath(u)
			}
			if u := strings.TrimSpace(os.Getenv("RENDER_EXTERNAL_URL")); u != "" {
				return ensureHealthPath(u)
			}
			if u := strings.TrimSpace(os.Getenv("RENDER_URL")); u != "" {
				return ensureHealthPath(u)
			}
			port := os.Getenv("PORT")
			if port == "" {
				port = "8080"
			}
			return fmt.Sprintf("http://localhost:%s/health", port)
		}

		client := &http.Client{Timeout: 8 * time.Second}

		// с джиттером, чтобы избежать синхронности между инстансами
		randomJitter := func(base time.Duration) time.Duration {
			// ±20% джиттер
			j := int64(base) / 5
			n := time.Now().UnixNano()
			// псевдорандом без глобального источника
			delta := (n % (2*j)) - j
			return base + time.Duration(delta)
		}

		// немедленный пинг при старте
		func() {
			url := resolveURL()
			retryPing(client, url)
		}()

		for {
			// Ждём с джиттером
			time.Sleep(randomJitter(pingInterval))
			
			url := resolveURL()
			retryPing(client, url)
		}
	}()
}

// retryPing выполняет до 3 попыток с экспоненциальной задержкой
func retryPing(client *http.Client, url string) {
	maxAttempts := 3
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		req, _ := http.NewRequest(http.MethodGet, url, nil)
		req.Header.Set("User-Agent", "keepalive/1.0")
		req.Header.Set("X-Render-Keep-Alive", "1")
		resp, err := client.Do(req)
		if err == nil {
			status := resp.StatusCode
			resp.Body.Close()
			if status == http.StatusOK {
				if attempt == 1 {
					log.Printf("✅ Самопинг успешен: %s", url)
				} else {
					log.Printf("✅ Самопинг успешен после %d попыток: %s", attempt, url)
				}
				return
			}
			log.Printf("⚠️ Самопинг статус %d (попытка %d/%d) для %s", status, attempt, maxAttempts, url)
		} else {
			log.Printf("❌ Ошибка самопинга (попытка %d/%d): %v (URL: %s)", attempt, maxAttempts, err, url)
		}
		time.Sleep(time.Duration(attempt*attempt) * time.Second)
	}
}

func handleMessage(bot *tgbotapi.BotAPI, message *tgbotapi.Message) {
	chatID := message.Chat.ID

	switch message.Text {
	case "/start":
		// Сброс состояний: у менеджера — полностью, у клиента — только клиентские
		if isManagerUser(message.From) {
			clearManagerStates(chatID)
		} else {
			clearClientStates(chatID)
		}
		// Стартовая точка: показ админ-панели админу
		// Проверяем, является ли пользователь менеджером
		if isManagerResponse(message) {
			sendManagerMenu(bot, chatID)
		} else {
			sendMainMenu(bot, chatID)
		}
	case "/manager":
		// Принудительно открыть меню менеджера, если у пользователя есть права менеджера
		if isManagerUser(message.From) {
			sendManagerMenu(bot, chatID)
		} else {
			msg := tgbotapi.NewMessage(chatID, "Недостаточно прав. Доступно только для менеджеров.")
			bot.Send(msg)
			sendMainMenu(bot, chatID)
		}
        if isAdminUser(message.From) {
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
		// Сбор имени для контакта с менеджером
		if nameCollectState[chatID] {
			if strings.TrimSpace(strings.ToLower(message.Text)) == "/cancel" {
				delete(nameCollectState, chatID)
				msg := tgbotapi.NewMessage(chatID, "❌ Отменено")
				bot.Send(msg)
				sendMainMenu(bot, chatID)
				return
			}

			providedName := strings.TrimSpace(message.Text)
			if providedName == "" {
				bot.Send(tgbotapi.NewMessage(chatID, "Пожалуйста, укажите имя"))
				return
			}

			// Убедимся, что есть тикет
			if _, exists := userTickets[chatID]; !exists {
				createTicketAndAskQuestion(bot, chatID, "Не определен")
			}
            if ticketID, ok := userTickets[chatID]; ok {
                updateTicketUserInfo(ticketID, message.From.UserName, providedName, "", "")
            }
			delete(nameCollectState, chatID)
			bot.Send(tgbotapi.NewMessage(chatID, "Спасибо! Теперь напишите ваш вопрос менеджеру."))
			// Включаем режим диалога
			questionStates[chatID] = true
			return
		}
		// Обработка поиска тикетов для менеджеров
		if isManagerUser(message.From) {
			if handleTicketSearchInput(bot, message) || handleExportTicketIDInput(bot, message) {
				return
			}
		}
		// Обработка ввода для админ-операций, если активен режим
        if isAdminUser(message.From) {
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
		// Переназначаем поведение для менеджеров: возвращаем в менеджерское меню
		log.Printf("Возврат в главное меню для чата %d", chatID)
		if isManagerUser(callback.From) {
			sendManagerMenu(bot, chatID)
		} else {
			sendMainMenu(bot, chatID)
		}
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
	case "manager_export_users_xml":
		if isManagerUser(callback.From) {
			if buf, err := exportUsersXML(); err == nil {
				sendExcelBuffer(bot, chatID, "users.xml", buf)
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
    case "survey_for_self_yes":
        if st, ok := userStates[chatID]; ok {
            v := true
            st.ForSelf = &v
            st.Step = 3
            bot.Send(tgbotapi.NewMessage(chatID, "Ваш рост? (в см)"))
        }
    case "survey_for_self_no":
        if st, ok := userStates[chatID]; ok {
            v := false
            st.ForSelf = &v
            st.Step = 3
            bot.Send(tgbotapi.NewMessage(chatID, "Ваш рост? (в см)"))
        }
	case "catalog":
		if isManagerUser(callback.From) {
			showCatalogForManager(bot, chatID)
		} else {
			showCatalog(bot, chatID)
		}
    case "help":
        if isManagerUser(callback.From) {
            handleManagerHelpCallback(bot, chatID)
        } else {
            sendMainMenu(bot, chatID)
        }
    case "admin_panel":
        if isAdminUser(callback.From) {
            showAdminPanel(bot, chatID)
        } else {
            sendMainMenu(bot, chatID)
        }
    case "admin_list_managers":
        if isAdminUser(callback.From) {
            showManagersList(bot, chatID)
        }
    case "admin_add_manager":
        if isAdminUser(callback.From) {
            promptAddManager(bot, chatID)
        }
    case "admin_remove_manager":
        if isAdminUser(callback.From) {
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
		nameCollectState[chatID] = true
		prompt := tgbotapi.NewMessage(chatID, "Как к вам обращаться? Укажите имя.\n\nИспользуйте /cancel для отмены.")
		bot.Send(prompt)
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
            if isAdminUser(callback.From) {
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
// calculateSize больше не используется (заменено на getSizeInfo)

// Функция для запуска опроса о товарах
func startSurvey(bot *tgbotapi.BotAPI, chatID int64) {
	log.Printf("Начинаю опрос для чата %d", chatID)
    userStates[chatID] = &UserState{Step: 1}

    // Создаём тикет для сохранения данных опроса
    if _, exists := userTickets[chatID]; !exists {
        createTicketAndAskQuestion(bot, chatID, "Не определен")
    }

    // Шаг 1: запрос ФИО
    msg := tgbotapi.NewMessage(chatID, "Введите ваше ФИО (Фамилия Имя Отчество). Если отчества нет, напишите только фамилию и имя.")
	if _, err := bot.Send(msg); err != nil {
		log.Printf("Ошибка отправки сообщения: %v", err)
		return
	}
}

// Функция для обработки ответов в опросе
func handleSurveyResponse(bot *tgbotapi.BotAPI, message *tgbotapi.Message, state *UserState) {
	chatID := message.Chat.ID

	switch state.Step {
    case 1:
        // Ожидаем: ФИО (Фамилия Имя Отчество?)
        full := strings.TrimSpace(message.Text)
        if full == "" {
            bot.Send(tgbotapi.NewMessage(chatID, "Пожалуйста, укажите ФИО"))
            return
        }
        parts := strings.Fields(full)
        if len(parts) < 2 {
            bot.Send(tgbotapi.NewMessage(chatID, "Укажите минимум фамилию и имя"))
            return
        }
        // Интерпретируем как: Фамилия Имя [Отчество]
        state.LastName = parts[0]
        state.FirstName = parts[1]
        if len(parts) > 2 {
            state.DopName = strings.Join(parts[2:], " ")
        }
        state.Step = 2
        // Шаг 2: выбираете ли вы для себя?
        prompt := tgbotapi.NewMessage(chatID, "Выбираете ли вы для себя?")
        prompt.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
            tgbotapi.NewInlineKeyboardRow(
                tgbotapi.NewInlineKeyboardButtonData("Да", "survey_for_self_yes"),
                tgbotapi.NewInlineKeyboardButtonData("Нет", "survey_for_self_no"),
            ),
        )
        bot.Send(prompt)
        return
    case 2:
        // Здесь ждём нажатия инлайн-кнопки; если пришёл текст — игнорируем
        bot.Send(tgbotapi.NewMessage(chatID, "Нажмите кнопку: Да/Нет"))
        return
    case 3:
		height, err := strconv.Atoi(message.Text)
		if err != nil {
			msg := tgbotapi.NewMessage(chatID, "Пожалуйста, введите рост в сантиметрах (например: 175)")
			bot.Send(msg)
			return
		}

		if height < 150 || height > 220 {
			msg := tgbotapi.NewMessage(chatID, "Рост должен быть от 150 до 220 см. Попробуйте еще раз:")
			bot.Send(msg)
			return
		}

        state.Height = height
        state.Step = 4
        msg := tgbotapi.NewMessage(chatID, "Ваш размер груди? (в см)")
		bot.Send(msg)
    case 4:
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
        state.Step = 5
        askOversizeQuestion(bot, chatID)
        return

    case 5:
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
        // Сохраняем имя/фамилию в тикет (если уже создан)
        if ticketID, exists := userTickets[chatID]; exists {
            updateTicketUserInfo(ticketID, message.From.UserName, state.FirstName, state.LastName, state.DopName)
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

    // Автовыбор модели по ответу об оверсайзе: если Да — берём модель с поддержкой оверсайз (индекс 0),
    // если Нет — берём обычную модель (индекс 1 при наличии, иначе 0)
    teeIndex := 0
    if state.Oversize {
        teeIndex = 0
    } else if len(products) > 1 {
        teeIndex = 1
    }

    product := products[teeIndex]

	// Оверсайз только для модели "Крылатые Фразы" (индекс 0)
    oversize := state.Oversize && teeIndex == 0
	mark, ru := getSizeInfo(state.ChestSize, oversize)

	heightInfo := ""
	if state.Height > 0 {
		var hRange string
		if state.Height >= 158 && state.Height <= 175 {
			hRange = "158-175"
		} else if state.Height >= 176 && state.Height <= 188 {
			hRange = "176-188"
		}
		if hRange != "" {
			heightInfo = fmt.Sprintf("\nРост: %s см", hRange)
		}
	}

	responseText := fmt.Sprintf("Вам подойдут следующие размеры модели:\n\n%s\nМаркировка: %s\nРоссийский размер: %s%s",
		product.Name, mark, ru, heightInfo)

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
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Связаться с менеджером", "contact_manager"),
		),
	)

	msg.ReplyMarkup = keyboard
	if _, err := bot.Send(msg); err != nil {
		log.Printf("Ошибка отправки рекомендаций: %v", err)
	}

    // Пробуем записать данные в активный тикет пользователя (если есть)
    if ticketID, exists := userTickets[chatID]; exists {
        if t, ok := tickets[ticketID]; ok {
            // Сохраняем данные опроса в тикет
            t.Height = state.Height
            t.ChestSize = state.ChestSize
            t.Oversize = oversize
            
            // Сохраняем рекомендованный размер
            mark, _ := getSizeInfo(state.ChestSize, oversize)
            t.RecommendedSize = mark
            
            // если ФИО было собрано — обновим
            if state.FirstName != "" || state.LastName != "" || state.DopName != "" {
                t.FirstName = state.FirstName
                t.LastName = state.LastName
                t.DopName = state.DopName
            }
            t.LastMessage = time.Now()
            saveTickets()
        }
    }
}

// getSizeInfo возвращает маркировку и российский размер по таблице, учитывая оверсайз
func getSizeInfo(chestSize int, oversize bool) (string, string) {
	type row struct{ mark, ru string }
	table := []struct {
		min  int
		max  int
		size row
	}{
		{82, 89, row{"XS-S", "42-44"}},
		{90, 97, row{"M-L", "46-48"}},
		{98, 105, row{"XL-2XL", "50-52"}},
		{106, 113, row{"3XL-4XL", "54-56"}},
		{114, 121, row{"5XL-6XL", "58-60"}},
	}
	idx := -1
	for i, r := range table {
		if chestSize >= r.min && chestSize <= r.max {
			idx = i
			break
		}
	}
	if idx == -1 {
		if chestSize < table[0].min {
			idx = 0
		} else if chestSize > table[len(table)-1].max {
			idx = len(table) - 1
		}
	}
	if oversize && idx < len(table)-1 {
		idx++
	}
	return table[idx].size.mark, table[idx].size.ru
}

// isWithinBusinessHours проверяет, попадает ли текущее локальное время в 09:00-20:00
func isWithinBusinessHours() bool {
	now := time.Now()
	h := now.Hour()
	return h >= 9 && h < 20
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

	// Меню после каталога: подбор и связь с менеджером
	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Подобрать", "select"),
			tgbotapi.NewInlineKeyboardButtonData("Связаться с менеджером", "contact_manager"),
		),
	)

	finalMsg := tgbotapi.NewMessage(chatID, "Выберите действие:")
	finalMsg.ReplyMarkup = keyboard
	if _, err := bot.Send(finalMsg); err != nil {
		log.Printf("Ошибка отправки финального сообщения каталога: %v", err)
	}
}

// showCatalogForManager — версия каталога без клиентских CTA
func showCatalogForManager(bot *tgbotapi.BotAPI, chatID int64) {
	log.Printf("Показываю каталог (менеджер) для чата %d", chatID)

	msg := tgbotapi.NewMessage(chatID, "Каталог товаров:")
	if _, err := bot.Send(msg); err != nil {
		log.Printf("Ошибка отправки сообщения каталога: %v", err)
		return
	}

	for _, product := range products {
		photo := tgbotapi.NewPhoto(chatID, tgbotapi.FilePath(product.ImageURL))
		photo.Caption = fmt.Sprintf("%s\nРазмеры: %s\nСсылка на сайт: [%s](%s)",
			product.Name, strings.Join(product.Sizes, ", "), product.Name, product.Link)
		photo.ParseMode = "MarkdownV2"
		if _, err := bot.Send(photo); err != nil {
			log.Printf("Ошибка отправки фото каталога для %s: %v, отправляю текстовое сообщение", product.Name, err)
			textMsg := tgbotapi.NewMessage(chatID, fmt.Sprintf("%s\nРазмеры: %s\nСсылка на сайт: [%s](%s)",
				product.Name, strings.Join(product.Sizes, ", "), product.Name, product.Link))
			textMsg.ParseMode = "MarkdownV2"
			if _, textErr := bot.Send(textMsg); textErr != nil {
				log.Printf("Ошибка отправки текстового сообщения каталога: %v", textErr)
			}
		}
	}

	// Только возврат в меню менеджера
	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🔙 Назад", "back_to_manager_menu"),
		),
	)
	finalMsg := tgbotapi.NewMessage(chatID, "Готово. Вернуться в меню менеджера:")
	finalMsg.ReplyMarkup = keyboard
	if _, err := bot.Send(finalMsg); err != nil {
		log.Printf("Ошибка отправки финального сообщения каталога (менеджер): %v", err)
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
    updateTicketUserInfo(ticketID, message.From.UserName, message.From.FirstName, message.From.LastName, "")

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

	// Подтверждаем клиенту + уведомление о времени ответа
	confirmText := "✅ Сообщение отправлено менеджеру!"
	if !isWithinBusinessHours() {
		confirmText += "\n\n⏰ Менеджер отвечает с 09:00 до 20:00. Он свяжется с вами в это время."
	}
	confirmMsg := tgbotapi.NewMessage(chatID, confirmText)
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
