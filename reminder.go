package main

import (
	"log"
	"strings"
	"telecho/database"
	"telecho/logger"
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
	bot            *tg.BotAPI
	db             *database.Database
	reminderToUser map[int]*ReminderSet
}

const (
	clock          = 20 * time.Second
	barrier12Hours = 12 * time.Hour
)

var reminderMaster ReminderMaster

func InitReminders(bot *tg.BotAPI, db *database.Database) {
	reminderMaster = ReminderMaster{
		bot:            bot,
		db:             db,
		reminderToUser: make(map[int]*ReminderSet),
	}

	ch := time.NewTicker(clock).C
	go remind(ch)
}

func remind(ch <-chan time.Time) {
	for range ch {
		hours, minutes, _ := time.Now().Clock()
		t := hours*60 + minutes
		r, ok := reminderMaster.reminderToUser[t]
		if !ok {
			continue
		}

		now := time.Now()
		for u, r := range r.reminders {
			if now.Sub(r.lastSentOn) > barrier12Hours {
				r.lastSentOn = now
				log.Printf("Sending a reminder for user %d", u)
				go sendReminder(u, r, &reminderMaster)
			}
		}
	}
}

func sendReminder(u int64, r *Reminder, rm *ReminderMaster) {
	db := (*(*rm).db)

	list := db.ListFirstMemos(u, r.chatID, 5, true)
	if _, err := sendMemosForToday(rm.bot, r.chatID, &list, nil); err != nil {
		logger.ForUser(u, "failed on '/list:'", err)
	}
}

// func delReminder(u int64) {
// 	data := db[u]
// 	t := data.Config.RemindHour*60 + data.Config.RemindMin

// 	reminderSet := reminderMaster.reminderSet[t]
// 	if reminderSet == nil {
// 		return
// 	}
// 	if reminderSet.reminders == nil {
// 		return
// 	}

// 	// reminderSet.reminders
// }

func setReminder(u int64) bool {
	rp, ok := reminderMaster.db.GetRemindParams(u)
	if !ok {
		return false
	}

	if rp == nil {
		logger.ForUser(u, "no reminder parameters found", nil)
		return false
	}

	tUTC := rp.RemindAt.UTC()
	t := tUTC.Hour()*60 + tUTC.Minute()

	r2u := reminderMaster.reminderToUser[t]
	if r2u == nil {
		r2u = &ReminderSet{reminders: make(map[int64]*Reminder)}
		reminderMaster.reminderToUser[t] = r2u
	}
	if r2u.reminders == nil {
		r2u.reminders = make(map[int64]*Reminder)
	}

	r2u.reminders[u] = &Reminder{
		lastSentOn: time.UnixMicro(0),
		userID:     u,
		chatID:     rp.ChatID,
	}

	return true
}

func updateReminder(bot *tg.BotAPI, db *database.Database, u, chatID int64, text string) bool {
	parts := strings.Split(text, ":")
	if len(parts) != 2 {
		return false
	}

	hour, err := validateInt(parts[0], 0, 23)
	if err != nil {
		return false
	}

	minute, err := validateInt(parts[1], 0, 59)
	if err != nil {
		return false
	}

	rp, ok := db.GetRemindParams(u)
	if !ok {
		logger.ForUser(u, "failed to update reminder", nil)
		return false
	}
	if rp == nil {
		logger.ForUser(u, "no reminder parameters found", nil)
		return false
	}

	t := time.Date(0, 0, 0, hour, minute, 0, 0, rp.RemindAt.Location())
	ok = db.SetRemindAt(u, chatID, t)
	if !ok {
		return false
	}

	return setReminder(u)
}
