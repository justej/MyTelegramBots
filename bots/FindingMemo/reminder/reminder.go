package reminder

import (
	"botfarm/bots/FindingMemo/db"
	"container/heap"
	"time"

	"github.com/jmhodges/clock"
	"github.com/pkg/errors"
	"go.uber.org/zap"
)

const reminderTick = 20 * time.Second

var (
	clk = clock.New()
)

type Manager struct {
	db            *db.Database
	logger        *zap.SugaredLogger
	reminderQueue *reminderQueue
}

type Reminder struct {
	logger       *zap.SugaredLogger
	at           time.Time
	usr          int64
	sendReminder func(int64)
}

func NewManager(d *db.Database, sr func(int64), l *zap.SugaredLogger) *Manager {
	return &Manager{
		db:            d,
		logger:        l,
		reminderQueue: NewReminderQueue(),
	}
}

func (r *Manager) Run() {
	ch := time.NewTicker(reminderTick).C

	users, err := r.db.GetUsers()
	if err != nil {
		r.logger.Fatalw("failed getting list of users", "err", err)
	}

	r.logger.Infof("initializing reminders for %d users", len(users))

	for _, usr := range users {
		err = r.Set(usr)
		if err != nil {
			r.logger.Errorw("failed to fetch remind parameters; the user won't get reminders", "err", err)
		}
	}

	go remind(ch, r.reminderQueue)
}

func (r *Manager) Set(usr int64) error {
	rp, err := r.db.GetRemindParams(usr)
	if err != nil {
		return errors.Wrap(err, "failed getting reminder parameters")
	}

	if rp == nil {
		return errors.New("no reminder parameters found")
	}

	// TODO: add location cache
	loc, err := time.LoadLocation(rp.TimeZone)
	if err != nil {
		r.logger.Errorw("failed loading location; using UTC time zone", "err", err)
		loc = time.UTC
	}

	h := rp.RemindAt / 60
	m := rp.RemindAt - 60*h
	now := clk.Now().In(loc)

	// TODO: compare current time with last seen
	if (h < now.Hour()) || (h == now.Hour() && m <= now.Minute()) {
		now = now.Add(24 * time.Hour)
	}

	reminder := &Reminder{
		usr: usr,
		at:  time.Date(now.Year(), now.Month(), now.Day(), h, m, 0, 0, loc).UTC(),
	}

	heap.Push(r.reminderQueue, reminder)

	return nil
}

func remind(ch <-chan time.Time, reminderQueue *reminderQueue) {
	for range ch {
		now := clk.Now().UTC()
		for {
			r, ok := reminderQueue.Peek().(*Reminder)
			if !ok || now.Before(r.at) {
				break
			}

			heap.Pop(reminderQueue)

			// reminder doesn't have user in its context, so adding it now
			r.logger.Info("reminder is being sent")

			go r.sendReminder(r.usr)
			r.at = r.at.Add(24 * time.Hour)
			heap.Push(reminderQueue, r)
		}
	}
}
