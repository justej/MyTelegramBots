package reminder

import (
	"botfarm/bot"
	"botfarm/bots/FindingMemo/db"
	"container/heap"
	"log"
	"time"

	"github.com/jmhodges/clock"
)

const reminderTick = 20 * time.Second

var (
	clk          = clock.New()
	rq           = NewReminderQueue()
	sendReminder func(*bot.Context, int64, int64)
)

type Reminder struct {
	at     time.Time
	chatID int64
	userID int64
}

func Init(ctx *bot.Context, s func(*bot.Context, int64, int64)) {
	sendReminder = s
	ch := time.NewTicker(reminderTick).C

	users := db.GetUsers(ctx)

	log.Printf("Initializing reminders for %d users", len(users))

	for _, u := range users {
		ok := Set(ctx, u)
		if !ok {
			ctx.Logger.Error("failed to fetch remind parameters; the user won't get reminders")
		}
	}

	go remind(ctx, ch)
}

func remind(ctx *bot.Context, ch <-chan time.Time) {
	for range ch {
		now := clk.Now().UTC()
		for {
			r, ok := rq.Peek().(*Reminder)
			if !ok || now.Before(r.at) {
				break
			}

			heap.Pop(rq)

			log.Printf("sending a reminder for user %d", r.userID)
			go sendReminder(ctx, r.userID, r.chatID)
			r.at = r.at.Add(24 * time.Hour)
			heap.Push(rq, r)
		}
	}
}

func delReminder(u int64) {
	rq.Delete(u)
}

func Set(ctx *bot.Context, u int64) bool {
	rp, ok := db.GetRemindParams(ctx, u)
	if !ok {
		return false
	}

	if rp == nil {
		ctx.Logger.Warn("no reminder parameters found")
		return false
	}

	// TODO: add location cache
	loc, err := time.LoadLocation(rp.TimeZone)
	if err != nil {
		ctx.Logger.Errorw("failed loading location; using UTC time zone", "err", err)
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
