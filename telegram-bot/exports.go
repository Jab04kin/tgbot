package main

import (
	"bytes"
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

	headers := []string{"UserID", "Username", "FirstName", "LastName", "TicketsCount", "OpenTickets", "ClosedTickets", "LastMessageAt"}
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(sheet, cell, h)
	}

	// агрегируем по пользователям
	type userAgg struct {
		UserID        int64
		Username      string
		FirstName     string
		LastName      string
		TicketsCount  int
		OpenTickets   int
		ClosedTickets int
		LastMessageAt time.Time
	}
	agg := map[int64]*userAgg{}
	for _, t := range tickets {
		ua, ok := agg[t.UserID]
		if !ok {
			ua = &userAgg{UserID: t.UserID}
			agg[t.UserID] = ua
		}
		if t.Username != "" {
			ua.Username = t.Username
		}
		if t.FirstName != "" {
			ua.FirstName = t.FirstName
		}
		if t.LastName != "" {
			ua.LastName = t.LastName
		}
		ua.TicketsCount++
		switch t.Status {
		case "open":
			ua.OpenTickets++
		case "closed":
			ua.ClosedTickets++
		}
		if t.LastMessage.After(ua.LastMessageAt) {
			ua.LastMessageAt = t.LastMessage
		}
	}

	// отсортируем по LastMessageAt desc
	rows := make([]*userAgg, 0, len(agg))
	for _, v := range agg {
		rows = append(rows, v)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].LastMessageAt.After(rows[j].LastMessageAt) })

	for r, ua := range rows {
		rowIdx := r + 2
		f.SetCellValue(sheet, fmt.Sprintf("A%d", rowIdx), ua.UserID)
		f.SetCellValue(sheet, fmt.Sprintf("B%d", rowIdx), ua.Username)
		f.SetCellValue(sheet, fmt.Sprintf("C%d", rowIdx), ua.FirstName)
		f.SetCellValue(sheet, fmt.Sprintf("D%d", rowIdx), ua.LastName)
		f.SetCellValue(sheet, fmt.Sprintf("E%d", rowIdx), ua.TicketsCount)
		f.SetCellValue(sheet, fmt.Sprintf("F%d", rowIdx), ua.OpenTickets)
		f.SetCellValue(sheet, fmt.Sprintf("G%d", rowIdx), ua.ClosedTickets)
		f.SetCellValue(sheet, fmt.Sprintf("H%d", rowIdx), ua.LastMessageAt.Format("2006-01-02 15:04:05"))
	}

	buf, err := f.WriteToBuffer()
	if err != nil {
		return nil, err
	}
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

func sendExcelBuffer(bot *tgbotapi.BotAPI, chatID int64, filename string, buf *bytes.Buffer) {
	fileBytes := tgbotapi.FileBytes{
		Name:  filename,
		Bytes: buf.Bytes(),
	}
	doc := tgbotapi.NewDocument(chatID, fileBytes)
	bot.Send(doc)
}
