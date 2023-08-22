package reminder

import (
	"container/heap"
	"findingmemo/database"
	"findingmemo/logger"
	"log"
	"time"

	"github.com/jmhodges/clock"
)

const reminderTick = 20 * time.Second

var (
	clk          = clock.New()
	rq           = NewReminderQueue()
	db           *database.Database
	sendReminder func(int64, int64)
)

type Reminder struct {
	at     time.Time
	chatID int64
	userID int64
}

func Init(d *database.Database, s func(int64, int64)) {
	db = d
	sendReminder = s
	ch := time.NewTicker(reminderTick).C

	users := db.GetUsers()

	log.Printf("Initializing reminders for %d users", len(users))

	for _, u := range users {
		ok := Set(u)
		if !ok {
			logger.ForUser(u, "failed to fetch remind parameters; the user won't get reminders", nil)
		}
	}

	go remind(ch)
}

func remind(ch <-chan time.Time) {
	for range ch {
		now := clk.Now().UTC()
		for {
			r, ok := rq.Peek().(*Reminder)
			if !ok || now.Before(r.at) {
				break
			}

			heap.Pop(rq)

			log.Printf("sending a reminder for user %d", r.userID)
			go sendReminder(r.userID, r.chatID)
			r.at = r.at.Add(24 * time.Hour)
			heap.Push(rq, r)
		}
	}
}

func delReminder(u int64) {
	rq.Delete(u)
}

func Set(u int64) bool {
	rp, ok := db.GetRemindParams(u)
	if !ok {
		return false
	}

	if rp == nil {
		logger.ForUser(u, "no reminder parameters found", nil)
		return false
	}

	// TODO: add location cache
	loc, err := time.LoadLocation(rp.TimeZone)
	if err != nil {
		logger.ForUser(u, "failed loading location; using UTC time zone", err)
		loc = time.UTC
	}

	h := rp.RemindAt / 60
	m := rp.RemindAt - 60*h
	now := clk.Now().In(loc)

	// TODO: compare current time with last seen

	if (h < now.Hour()) || (h == now.Hour() && m <= now.Minute()) {
		now = now.Add(24 * time.Hour)
	}

	r := &Reminder{
		userID: u,
		chatID: rp.ChatID,
		at:     time.Date(now.Year(), now.Month(), now.Day(), h, m, 0, 0, loc).UTC(),
	}

	heap.Push(rq, r)

	return true
}
