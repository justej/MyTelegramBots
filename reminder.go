package main

import (
	"errors"
	"log"
	"strings"
	"telecho/database"
	"time"

	tg "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type Reminder struct {
	lastSentOn time.Time // when reminder was sent
	userID     int64
	chatID     int64
}

type ReminderSet struct {
	lastSentOn time.Time           // when all reminders were sent
	reminders  map[int64]*Reminder // set of reminders
}

type ReminderMaster struct {
	bot         *tg.BotAPI
	db          *database.Database
	reminderSet map[int]*ReminderSet
}

var reminderMaster ReminderMaster

func InitReminders(bot *tg.BotAPI, db *database.Database) {
	reminderMaster = ReminderMaster{
		bot:         bot,
		db:          db,
		reminderSet: make(map[int]*ReminderSet),
	}

	ch := time.NewTicker(20 * time.Second).C
	go remind(ch)
}

func remind(ch <-chan time.Time) {
	for range ch {
		hours, minutes, _ := time.Now().Clock()
		t := hours*60 + minutes
		r, ok := reminderMaster.reminderSet[t]
		if !ok {
			continue
		}

		now := time.Now()
		for u, r := range r.reminders {
			if now.Sub(r.lastSentOn) > 12*time.Hour {
				r.lastSentOn = now
				log.Printf("Sending a reminder for user %d", u)
				go sendReminder(u, r, &reminderMaster)
			}
		}
	}
}

func sendReminder(u int64, r *Reminder, rm *ReminderMaster) {
	db := (*(*rm).db)

	list := db.ListFirstMemos(u, 5, true)
	if _, err := sendMemosForToday(rm.bot, r.chatID, list); err != nil {
		logForUser(u, "failed on '/list:'", err)
	}
}

func delReminder(u int64) {
	data := db[u]
	t := data.Config.RemindHour*60 + data.Config.RemindMin

	reminderSet := reminderMaster.reminderSet[t]
	if reminderSet == nil {
		return
	}
	if reminderSet.reminders == nil {
		return
	}

	// reminderSet.reminders
}

func setReminder(u int64) {
	data := db[u]
	t := data.Config.RemindHour*60 + data.Config.RemindMin

	reminderSet := reminderMaster.reminderSet[t]
	if reminderSet == nil {
		reminderSet = &ReminderSet{reminders: make(map[int64]*Reminder)}
		reminderMaster.reminderSet[t] = reminderSet
	}
	if reminderSet.reminders == nil {
		reminderSet.reminders = make(map[int64]*Reminder)
	}

	reminderSet.reminders[u] = &Reminder{
		lastSentOn: time.UnixMicro(0),
		userID:     u,
		chatID:     data.Config.ChatID,
	}
}

func updateReminder(bot *tg.BotAPI, u, chatID int64, text string) error {
	parts := strings.Split(text, ":")
	if len(parts) != 2 {
		return errors.New("Only one ':' separator is allowed")
	}

	hour, err := validateInt(parts[0], 0, 23)
	if err != nil {
		return err
	}

	minute, err := validateInt(parts[1], 0, 59)
	if err != nil {
		return err
	}

	data, ok := db[u]
	if !ok {
		log.Printf("data is missing for user %d", u)
		data = *database.NewData()
	}

	if data.Config == nil {
		log.Printf("config is missing for user %d", u)
		data.Config = database.NewConfig()
	}

	data.Config.RemindHour = hour
	data.Config.RemindMin = minute
	db[u] = data

	setReminder(u)

	return nil
}
