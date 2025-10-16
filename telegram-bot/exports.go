package main

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"sort"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/xuri/excelize/v2"
)

// exportUsersExcel формирует Excel со сводной информацией по пользователям (по данным тикетов)
func exportUsersExcel() (*bytes.Buffer, error) {
    f := excelize.NewFile()
    sheet := f.GetSheetName(0)

    // Требуемые столбцы заказчика
    headers := []string{"id", "username", "ФИО", "Рост", "Размер груди", "Оверсайз", "Рекомендованный размер", "Размеры для другого", "Рекомендованный размер для другого"}
    for i, h := range headers {
        cell, _ := excelize.CoordinatesToCellName(i+1, 1)
        f.SetCellValue(sheet, cell, h)
    }

    type userRow struct {
        UserID          int64
        Username        string
        FirstName       string
        LastName        string
        Height          int
        ChestSize       int
        Oversize        bool
        RecommendedSize string
        LastMessageAt   time.Time
        // "для другого"
        OtherHeight     int
        OtherChestSize  int
        OtherOversize   *bool
        RecommendedOtherSize string
    }
    // Берём по каждому пользователю данные из самого "свежего" тикета (по LastMessage)
    best := map[int64]*userRow{}
    for _, t := range tickets {
        ur, ok := best[t.UserID]
        if !ok {
            ur = &userRow{UserID: t.UserID}
            best[t.UserID] = ur
        }
        // если этот тикет новее — перезапишем агрегированные пользовательские поля
        if t.LastMessage.After(ur.LastMessageAt) {
            ur.LastMessageAt = t.LastMessage
            ur.Username = t.Username
            ur.FirstName = t.FirstName
            ur.LastName = t.LastName
            ur.Height = t.Height
            ur.ChestSize = t.ChestSize
            ur.Oversize = t.Oversize
            ur.RecommendedSize = strings.ToLower(strings.TrimSpace(t.RecommendedSize))
            ur.RecommendedOtherSize = strings.ToLower(strings.TrimSpace(t.RecommendedOtherSize))
            // другие
            ur.OtherHeight = t.OtherHeight
            ur.OtherChestSize = t.OtherChestSize
            if t.OtherHeight > 0 || t.OtherChestSize > 0 || t.OtherOversize {
                v := t.OtherOversize
                ur.OtherOversize = &v
            } else {
                ur.OtherOversize = nil
            }
        }
        // если тикет не новее, но в нём есть непустые поля, которых ещё нет — мягко донаполняем
        if ur.Username == "" && t.Username != "" { ur.Username = t.Username }
        if ur.FirstName == "" && t.FirstName != "" { ur.FirstName = t.FirstName }
        if ur.LastName == "" && t.LastName != "" { ur.LastName = t.LastName }
        if ur.Height == 0 && t.Height > 0 { ur.Height = t.Height }
        if ur.ChestSize == 0 && t.ChestSize > 0 { ur.ChestSize = t.ChestSize }
        if !ur.Oversize && t.Oversize { ur.Oversize = true }
        if ur.RecommendedSize == "" && strings.TrimSpace(t.RecommendedSize) != "" {
            ur.RecommendedSize = strings.ToLower(strings.TrimSpace(t.RecommendedSize))
        }
        if ur.RecommendedOtherSize == "" && strings.TrimSpace(t.RecommendedOtherSize) != "" {
            ur.RecommendedOtherSize = strings.ToLower(strings.TrimSpace(t.RecommendedOtherSize))
        }
        if ur.OtherHeight == 0 && t.OtherHeight > 0 { ur.OtherHeight = t.OtherHeight }
        if ur.OtherChestSize == 0 && t.OtherChestSize > 0 { ur.OtherChestSize = t.OtherChestSize }
        if ur.OtherOversize == nil && (t.OtherHeight > 0 || t.OtherChestSize > 0 || t.OtherOversize) {
            v := t.OtherOversize
            ur.OtherOversize = &v
        }
    }

    users := buildUsersAggregate()
    sort.Slice(users, func(i, j int) bool { return users[i].UserID < users[j].UserID })

    for r, u := range users {
        rowIdx := r + 2
        fio := strings.TrimSpace(strings.Join([]string{strings.TrimSpace(u.LastName), strings.TrimSpace(u.FirstName), strings.TrimSpace(u.DopName)}, " "))
        fio = strings.TrimSpace(strings.ReplaceAll(fio, "  ", " "))
        // Соберём строку "Размеры для другого" в требуемом формате
        var otherParts []string
        if u.OtherHeight > 0 { otherParts = append(otherParts, fmt.Sprintf("Рост: %d", u.OtherHeight)) }
        if u.OtherChestSize > 0 { otherParts = append(otherParts, fmt.Sprintf("Размер груди: %d", u.OtherChestSize)) }
        if u.OtherHeight > 0 || u.OtherChestSize > 0 || u.OtherOversize {
            yn := "Нет"
            if u.OtherOversize { yn = "Да" }
            otherParts = append(otherParts, fmt.Sprintf("Оверсайз: %s", yn))
        }
        otherCombined := strings.Join(otherParts, ", ")

        f.SetCellValue(sheet, fmt.Sprintf("A%d", rowIdx), u.UserID)
        f.SetCellValue(sheet, fmt.Sprintf("B%d", rowIdx), u.Username)
        f.SetCellValue(sheet, fmt.Sprintf("C%d", rowIdx), fio)
        f.SetCellValue(sheet, fmt.Sprintf("D%d", rowIdx), u.Height)
        f.SetCellValue(sheet, fmt.Sprintf("E%d", rowIdx), u.ChestSize)
        f.SetCellValue(sheet, fmt.Sprintf("F%d", rowIdx), func() string { if u.Oversize { return "Да" } else { return "Нет" } }())
        f.SetCellValue(sheet, fmt.Sprintf("G%d", rowIdx), u.RecommendedSize)
        f.SetCellValue(sheet, fmt.Sprintf("H%d", rowIdx), otherCombined)
        f.SetCellValue(sheet, fmt.Sprintf("I%d", rowIdx), u.RecommendedOtherSize)
    }

    buf, err := f.WriteToBuffer()
    if err != nil { return nil, err }
    return buf, nil
}

// exportAllTicketsExcel формирует Excel со всеми тикетами
func exportAllTicketsExcel() (*bytes.Buffer, error) {
	f := excelize.NewFile()
	sheet := f.GetSheetName(0)

	headers := []string{"TicketID", "Status", "UserID", "Username", "FirstName", "LastName", "Height", "Chest", "Oversize", "Recommended", "Question", "CreatedAt", "LastMessage"}
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(sheet, cell, h)
	}

	// стабильно по ID
	ids := make([]int, 0, len(tickets))
	for id := range tickets {
		ids = append(ids, id)
	}
	sort.Ints(ids)

	for r, id := range ids {
		t := tickets[id]
		rowIdx := r + 2
		f.SetCellValue(sheet, fmt.Sprintf("A%d", rowIdx), t.ID)
		f.SetCellValue(sheet, fmt.Sprintf("B%d", rowIdx), t.Status)
		f.SetCellValue(sheet, fmt.Sprintf("C%d", rowIdx), t.UserID)
		f.SetCellValue(sheet, fmt.Sprintf("D%d", rowIdx), t.Username)
		f.SetCellValue(sheet, fmt.Sprintf("E%d", rowIdx), t.FirstName)
		f.SetCellValue(sheet, fmt.Sprintf("F%d", rowIdx), t.LastName)
		f.SetCellValue(sheet, fmt.Sprintf("G%d", rowIdx), t.Height)
		f.SetCellValue(sheet, fmt.Sprintf("H%d", rowIdx), t.ChestSize)
		f.SetCellValue(sheet, fmt.Sprintf("I%d", rowIdx), t.Oversize)
		f.SetCellValue(sheet, fmt.Sprintf("J%d", rowIdx), t.RecommendedSize)
		f.SetCellValue(sheet, fmt.Sprintf("K%d", rowIdx), t.Question)
		f.SetCellValue(sheet, fmt.Sprintf("L%d", rowIdx), t.CreatedAt.Format("2006-01-02 15:04:05"))
		f.SetCellValue(sheet, fmt.Sprintf("M%d", rowIdx), t.LastMessage.Format("2006-01-02 15:04:05"))
	}

	// Настроим ширины и шапку
	_ = f.SetColWidth(sheet, "A", "A", 10)
	_ = f.SetColWidth(sheet, "B", "B", 10)
	_ = f.SetColWidth(sheet, "C", "C", 14)
	_ = f.SetColWidth(sheet, "D", "F", 18)
	_ = f.SetColWidth(sheet, "G", "H", 10)
	_ = f.SetColWidth(sheet, "I", "K", 18)
	_ = f.SetColWidth(sheet, "L", "M", 20)
	_ = f.SetPanes(sheet, &excelize.Panes{Freeze: true, Split: true, XSplit: 0, YSplit: 1})

	// Лист сообщений по всем тикетам
	msgSheet := "Messages"
	f.NewSheet(msgSheet)
	msgHeaders := []string{"TicketID", "#", "SenderID", "FromManager", "Time", "Text"}
	for i, h := range msgHeaders {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(msgSheet, cell, h)
	}
	r := 2
	msgIDs := make([]int, 0, len(tickets))
	for id := range tickets {
		msgIDs = append(msgIDs, id)
	}
	sort.Ints(msgIDs)
	for _, id := range msgIDs {
		t := tickets[id]
		for _, m := range t.Messages {
			f.SetCellValue(msgSheet, fmt.Sprintf("A%d", r), t.ID)
			f.SetCellValue(msgSheet, fmt.Sprintf("B%d", r), m.ID)
			f.SetCellValue(msgSheet, fmt.Sprintf("C%d", r), m.SenderID)
			f.SetCellValue(msgSheet, fmt.Sprintf("D%d", r), m.IsFromManager)
			f.SetCellValue(msgSheet, fmt.Sprintf("E%d", r), m.Time.Format("2006-01-02 15:04:05"))
			f.SetCellValue(msgSheet, fmt.Sprintf("F%d", r), strings.ReplaceAll(m.Text, "\n", " "))
			r++
		}
	}
	_ = f.SetColWidth(msgSheet, "A", "E", 14)
	_ = f.SetColWidth(msgSheet, "F", "F", 80)
	_ = f.SetPanes(msgSheet, &excelize.Panes{Freeze: true, Split: true, XSplit: 0, YSplit: 1})

	// Стили: перенос текста для колонки F (Text) и жирная шапка
	wrapStyle, _ := f.NewStyle(&excelize.Style{Alignment: &excelize.Alignment{WrapText: true, Vertical: "top"}})
	headerStyle, _ := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true}})
	// применяем к шапкам обоих листов
	_ = f.SetCellStyle(sheet, "A1", "M1", headerStyle)
	_ = f.SetCellStyle(msgSheet, "A1", "F1", headerStyle)
	// применяем перенос для всех ячеек текста F2:F{r-1}
	if r > 2 {
		_ = f.SetCellStyle(msgSheet, "F2", fmt.Sprintf("F%d", r-1), wrapStyle)
		// увеличим высоту строк для читабельности
		for i := 2; i < r; i++ {
			_ = f.SetRowHeight(msgSheet, i, 28)
		}
	}

	buf, err := f.WriteToBuffer()
	if err != nil {
		return nil, err
	}
	return buf, nil
}

// exportSingleTicketExcel формирует Excel по одному тикету (с сообщениями на втором листе)
func exportSingleTicketExcel(ticketID int) (*bytes.Buffer, error) {
	t, ok := tickets[ticketID]
	if !ok {
		return nil, fmt.Errorf("ticket %d not found", ticketID)
	}

	f := excelize.NewFile()
	mainSheet := f.GetSheetName(0)

	// Основная информация
	rows := [][]any{
		{"TicketID", t.ID},
		{"Status", t.Status},
		{"UserID", t.UserID},
		{"Username", t.Username},
		{"FirstName", t.FirstName},
		{"LastName", t.LastName},
		{"Height", t.Height},
		{"Chest", t.ChestSize},
		{"Oversize", t.Oversize},
		{"Recommended", t.RecommendedSize},
		{"Question", t.Question},
		{"CreatedAt", t.CreatedAt.Format("2006-01-02 15:04:05")},
		{"LastMessage", t.LastMessage.Format("2006-01-02 15:04:05")},
	}
	for i, row := range rows {
		f.SetCellValue(mainSheet, fmt.Sprintf("A%d", i+1), row[0])
		f.SetCellValue(mainSheet, fmt.Sprintf("B%d", i+1), row[1])
	}

	// Лист сообщений
	messagesSheet := "Messages"
	f.NewSheet(messagesSheet)
	msgHeaders := []string{"#", "SenderID", "FromManager", "Time", "Text"}
	for i, h := range msgHeaders {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(messagesSheet, cell, h)
	}
	for idx, m := range t.Messages {
		rowIdx := idx + 2
		f.SetCellValue(messagesSheet, fmt.Sprintf("A%d", rowIdx), m.ID)
		f.SetCellValue(messagesSheet, fmt.Sprintf("B%d", rowIdx), m.SenderID)
		f.SetCellValue(messagesSheet, fmt.Sprintf("C%d", rowIdx), m.IsFromManager)
		f.SetCellValue(messagesSheet, fmt.Sprintf("D%d", rowIdx), m.Time.Format("2006-01-02 15:04:05"))
		f.SetCellValue(messagesSheet, fmt.Sprintf("E%d", rowIdx), strings.ReplaceAll(m.Text, "\n", " "))
	}

	buf, err := f.WriteToBuffer()
	if err != nil {
		return nil, err
	}
	return buf, nil
}

// XML структуры для экспорта пользователей
type UsersXML struct {
	XMLName xml.Name     `xml:"users"`
	Users   []UserXML    `xml:"user"`
}

type UserXML struct {
	XMLName                xml.Name `xml:"user"`
	ID                     int64    `xml:"id"`
	Username               string   `xml:"username"`
	FIO                    string   `xml:"fio"`
	Height                 int      `xml:"height"`
	ChestSize              int      `xml:"chest_size"`
	Oversize               string   `xml:"oversize"`
	RecommendedSize        string   `xml:"recommended_size"`
	OtherSizes             string   `xml:"other_sizes"`
	OtherRecommendedSize   string   `xml:"other_recommended_size"`
}

// exportUsersXML формирует XML со сводной информацией по пользователям
func exportUsersXML() (*bytes.Buffer, error) {
	users := buildUsersAggregate()
	sort.Slice(users, func(i, j int) bool { return users[i].UserID < users[j].UserID })

	var xmlUsers []UserXML
	for _, u := range users {
		fio := strings.TrimSpace(strings.Join([]string{strings.TrimSpace(u.LastName), strings.TrimSpace(u.FirstName), strings.TrimSpace(u.DopName)}, " "))
		fio = strings.TrimSpace(strings.ReplaceAll(fio, "  ", " "))

		// Соберём строку "Размеры для другого" в требуемом формате
		var otherParts []string
		if u.OtherHeight > 0 { otherParts = append(otherParts, fmt.Sprintf("Рост: %d", u.OtherHeight)) }
		if u.OtherChestSize > 0 { otherParts = append(otherParts, fmt.Sprintf("Размер груди: %d", u.OtherChestSize)) }
		if u.OtherHeight > 0 || u.OtherChestSize > 0 || u.OtherOversize {
			yn := "Нет"
			if u.OtherOversize { yn = "Да" }
			otherParts = append(otherParts, fmt.Sprintf("Оверсайз: %s", yn))
		}
		otherCombined := strings.Join(otherParts, ", ")

		oversizeStr := "Нет"
		if u.Oversize { oversizeStr = "Да" }

		xmlUser := UserXML{
			ID:                   u.UserID,
			Username:             u.Username,
			FIO:                  fio,
			Height:               u.Height,
			ChestSize:            u.ChestSize,
			Oversize:             oversizeStr,
			RecommendedSize:      u.RecommendedSize,
			OtherSizes:           otherCombined,
			OtherRecommendedSize: u.RecommendedOtherSize,
		}
		xmlUsers = append(xmlUsers, xmlUser)
	}

	usersXML := UsersXML{Users: xmlUsers}
	
	var buf bytes.Buffer
	buf.WriteString(xml.Header)
	encoder := xml.NewEncoder(&buf)
	encoder.Indent("", "  ")
	if err := encoder.Encode(usersXML); err != nil {
		return nil, err
	}

	return &buf, nil
}

func sendExcelBuffer(bot *tgbotapi.BotAPI, chatID int64, filename string, buf *bytes.Buffer) {
	fileBytes := tgbotapi.FileBytes{
		Name:  filename,
		Bytes: buf.Bytes(),
	}
	doc := tgbotapi.NewDocument(chatID, fileBytes)
	bot.Send(doc)
}
