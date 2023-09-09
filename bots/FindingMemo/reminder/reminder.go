package reminder

import (
	"botfarm/bot"
	"botfarm/bots/FindingMemo/db"
	"container/heap"
	"time"

	"github.com/jmhodges/clock"
)

const reminderTick = 20 * time.Second

var (
	clk          = clock.New()
	rq           = NewReminderQueue()
	sendReminder func(*bot.Context, int64)
)

type Reminder struct {
	at  time.Time
	cht int64
	usr int64
}

func Init(ctx *bot.Context, s func(*bot.Context, int64)) {
	sendReminder = s
	ch := time.NewTicker(reminderTick).C

	users := db.GetUsers(ctx)

	ctx.Logger.Infof("initializing reminders for %d users", len(users))

	for _, usr := range users {
		ok := Set(ctx, usr)
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

			// reminder doesn't have user in its context, so adding it now
			ctx := ctx.CloneWith(r.usr)

			ctx.Logger.Info("reminder is being sent")

			go sendReminder(ctx, r.cht)
			r.at = r.at.Add(24 * time.Hour)
			heap.Push(rq, r)
		}
	}
}

func Set(ctx *bot.Context, usr int64) bool {
	rp, ok := db.GetRemindParams(ctx, usr)
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
		usr: usr,
		cht: rp.ChatID,
		at:  time.Date(now.Year(), now.Month(), now.Day(), h, m, 0, 0, loc).UTC(),
	}

	heap.Push(rq, r)

	return true
}
