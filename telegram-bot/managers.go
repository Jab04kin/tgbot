package main

import (
	"encoding/json"
	"log"
	"os"
	"strconv"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

var managerIDsSet = make(map[int64]bool)
var managerUsernamesSet = make(map[string]bool)
var adminIDsSet = make(map[int64]bool)
var adminUsernamesSet = make(map[string]bool)

const managersStoreFile = "managers.json"

type managersStore struct {
	ManagerIDs       []int64  `json:"manager_ids"`
	ManagerUsernames []string `json:"manager_usernames"`
}

// initManagers загружает список менеджеров из переменных окружения
// Поддерживаются варианты:
// - MANAGER_ID (legacy, одиночный ID)
// - MANAGER_IDS (через запятую: "123,456")
// - MANAGER_USERNAMES (через запятую: без @, например: "alice,bob")
func initManagers() {
	managerIDsSet = make(map[int64]bool)
	managerUsernamesSet = make(map[string]bool)
	// 1) Load from file
	loadManagersFromFile()

	// 2) Seed from env (legacy + lists)
	// Legacy одиночный ID
	if v := strings.TrimSpace(os.Getenv("MANAGER_ID")); v != "" && v != "0" {
		if id, err := strconv.ParseInt(v, 10, 64); err == nil {
			managerIDsSet[id] = true
		} else {
			log.Printf("Некорректный MANAGER_ID: %v", err)
		}
	}

	// Список ID
	if v := strings.TrimSpace(os.Getenv("MANAGER_IDS")); v != "" {
		parts := strings.Split(v, ",")
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			if id, err := strconv.ParseInt(p, 10, 64); err == nil {
				managerIDsSet[id] = true
			} else {
				log.Printf("Пропускаю некорректный MANAGER_IDS элемент '%s': %v", p, err)
			}
		}
	}

	// Список username (без @)
	if v := strings.TrimSpace(os.Getenv("MANAGER_USERNAMES")); v != "" {
		parts := strings.Split(v, ",")
		for _, p := range parts {
			u := strings.TrimSpace(strings.TrimPrefix(p, "@"))
			if u == "" {
				continue
			}
			managerUsernamesSet[strings.ToLower(u)] = true
		}
	}

	if len(managerIDsSet) == 0 && len(managerUsernamesSet) == 0 {
		log.Printf("Менеджеры не заданы (клиентский режим). Установите MANAGER_ID(S) и/или MANAGER_USERNAMES")
	}
	// Persist
	saveManagersToFile()
}

func initAdmins() {
	adminIDsSet = make(map[int64]bool)
	adminUsernamesSet = make(map[string]bool)
	if v := strings.TrimSpace(os.Getenv("ADMIN_ID")); v != "" && v != "0" {
		if id, err := strconv.ParseInt(v, 10, 64); err == nil {
			adminIDsSet[id] = true
		} else {
			log.Printf("Некорректный ADMIN_ID: %v", err)
		}
	}
	if v := strings.TrimSpace(os.Getenv("ADMIN_IDS")); v != "" {
		parts := strings.Split(v, ",")
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			if id, err := strconv.ParseInt(p, 10, 64); err == nil {
				adminIDsSet[id] = true
			} else {
				log.Printf("Пропускаю некорректный ADMIN_IDS элемент '%s': %v", p, err)
			}
		}
	}
	if v := strings.TrimSpace(os.Getenv("ADMIN_USERNAMES")); v != "" {
		parts := strings.Split(v, ",")
		for _, p := range parts {
			u := strings.TrimSpace(strings.TrimPrefix(p, "@"))
			if u == "" {
				continue
			}
			adminUsernamesSet[strings.ToLower(u)] = true
		}
	}
}

func isManagerID(userID int64) bool {
	return managerIDsSet[userID]
}

func isManagerUsername(username string) bool {
	if username == "" {
		return false
	}
	return managerUsernamesSet[strings.ToLower(username)]
}

// isManagerUser проверяет пользователя по ID или username
func isManagerUser(user *tgbotapi.User) bool {
	if user == nil {
		return false
	}
	if isManagerID(user.ID) {
		return true
	}
	if isManagerUsername(user.UserName) {
		return true
	}
	return false
}

func isAdminUser(user *tgbotapi.User) bool {
	if user == nil {
		return false
	}
	if adminIDsSet[user.ID] {
		return true
	}
	if user.UserName != "" && adminUsernamesSet[strings.ToLower(user.UserName)] {
		return true
	}
	return false
}

func getManagerIDs() []int64 {
	ids := make([]int64, 0, len(managerIDsSet))
	for id := range managerIDsSet {
		ids = append(ids, id)
	}
	return ids
}

func getAdminIDs() []int64 {
	ids := make([]int64, 0, len(adminIDsSet))
	for id := range adminIDsSet {
		ids = append(ids, id)
	}
	return ids
}

func saveManagersToFile() {
	store := managersStore{}
	for id := range managerIDsSet {
		store.ManagerIDs = append(store.ManagerIDs, id)
	}
	for u := range managerUsernamesSet {
		store.ManagerUsernames = append(store.ManagerUsernames, u)
	}
	data, err := json.MarshalIndent(&store, "", "  ")
	if err != nil {
		log.Printf("Ошибка сериализации managers.json: %v", err)
		return
	}
	if err := os.WriteFile(managersStoreFile, data, 0644); err != nil {
		log.Printf("Ошибка записи %s: %v", managersStoreFile, err)
	}
}

func loadManagersFromFile() {
	data, err := os.ReadFile(managersStoreFile)
	if err != nil {
		return
	}
	var store managersStore
	if err := json.Unmarshal(data, &store); err != nil {
		log.Printf("Ошибка чтения %s: %v", managersStoreFile, err)
		return
	}
	for _, id := range store.ManagerIDs {
		managerIDsSet[id] = true
	}
	for _, u := range store.ManagerUsernames {
		if u == "" {
			continue
		}
		managerUsernamesSet[strings.ToLower(u)] = true
	}
}

func addManagerByID(userID int64) {
	managerIDsSet[userID] = true
	saveManagersToFile()
}

func removeManagerByID(userID int64) {
	delete(managerIDsSet, userID)
	saveManagersToFile()
}
