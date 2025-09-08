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
	Step        int
	SelectedTee string
	Height      int
	ChestSize   int
	Oversize    bool
}

type Product struct {
	Name     string
	Sizes    []string
	Link     string
	ImageURL string
}

var userStates = make(map[int64]*UserState)

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
		sendMainMenu(bot, chatID)
	default:
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
	case "bitrix24_line":
		log.Printf("Подключение к открытой линии Битрикс24 для чата %d", chatID)
		connectToBitrix24(bot, chatID)
	case "back_to_menu":
		log.Printf("Возврат в главное меню для чата %d", chatID)
		sendMainMenu(bot, chatID)
	case "oversize_yes":
		handleOversizeCallback(bot, chatID, true)
	case "oversize_no":
		handleOversizeCallback(bot, chatID, false)
	default:
		if strings.HasPrefix(callback.Data, "tee_") {
			selectedTee := strings.TrimPrefix(callback.Data, "tee_")
			log.Printf("Выбрана футболка %s для чата %d", selectedTee, chatID)
			startHeightQuestion(bot, chatID, selectedTee)
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

		if height < 100 || height > 300 {
			msg := tgbotapi.NewMessage(chatID, "Рост должен быть от 100 до 300 см. Попробуйте еще раз:")
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

		if chestSize < 30 || chestSize > 100 {
			msg := tgbotapi.NewMessage(chatID, "Обхват груди должен быть от 30 до 100 см. Попробуйте еще раз:")
			bot.Send(msg)
			return
		}

		state.ChestSize = chestSize
		state.Step = 4
		askOversizeQuestion(bot, chatID)

	case 4:
		response := strings.ToLower(message.Text)
		if response == "да" || response == "yes" {
			state.Oversize = true
		} else if response == "нет" || response == "no" {
			state.Oversize = false
		} else {
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
			tgbotapi.NewInlineKeyboardButtonData("Открытая линия Битрикс24", "bitrix24_line"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Назад в меню", "back_to_menu"),
		),
	)

	msg.ReplyMarkup = keyboard
	bot.Send(msg)
}

func connectToBitrix24(bot *tgbotapi.BotAPI, chatID int64) {
	// Здесь будет логика подключения к открытой линии Битрикс24
	// Пока что отправляем сообщение о подключении

	msg := tgbotapi.NewMessage(chatID, "🔗 Подключение к открытой линии Битрикс24...\n\nМенеджер скоро свяжется с вами через открытую линию.\n\nДля возврата в главное меню нажмите кнопку ниже.")

	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Главное меню", "back_to_menu"),
		),
	)

	msg.ReplyMarkup = keyboard
	bot.Send(msg)
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

	size := calculateSize(state.Height, state.ChestSize, state.Oversize)

	responseText := fmt.Sprintf("Вам подойдут следующие размеры модели:\n\n%s - размер %s",
		product.Name, size)

	msg := tgbotapi.NewMessage(chatID, responseText)

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

func calculateSize(height, chestSize int, oversize bool) string {
	var baseSize string

	if chestSize <= 85 {
		baseSize = "S"
	} else if chestSize <= 95 {
		baseSize = "M"
	} else if chestSize <= 105 {
		baseSize = "L"
	} else if chestSize <= 115 {
		baseSize = "XL"
	} else {
		baseSize = "XXL"
	}

	if oversize {
		switch baseSize {
		case "S":
			return "M"
		case "M":
			return "L"
		case "L":
			return "XL"
		case "XL":
			return "XXL"
		default:
			return "XXL"
		}
	}

	return baseSize
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
