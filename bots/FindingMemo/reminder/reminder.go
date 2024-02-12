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

func (m *Manager) Set(usr int64) error {
	rp, err := m.db.GetRemindParams(usr)
	if err != nil {
		return errors.Wrap(err, "failed getting reminder parameters")
	}

	if rp == nil {
		return errors.New("no reminder parameters found")
	}

	// TODO: add location cache
	loc, err := time.LoadLocation(rp.TimeZone)
	if err != nil {
		m.logger.Errorw("failed loading location; using UTC time zone", "err", err)
		loc = time.UTC
	}

	hh := rp.RemindAt / 60
	mm := rp.RemindAt - 60*hh
	now := clk.Now().In(loc)

	// TODO: compare current time with last seen
	if (hh < now.Hour()) || (hh == now.Hour() && mm <= now.Minute()) {
		now = now.Add(24 * time.Hour)
	}

	reminder := &Reminder{
		usr: usr,
		at:  time.Date(now.Year(), now.Month(), now.Day(), hh, mm, 0, 0, loc).UTC(),
	}

	heap.Push(m.reminderQueue, reminder)

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
