package database

import "time"

const (
	memoStateActive uint = iota
	memoStateDone
	memoStateDeleted
)

type memo struct {
	text     string    // memo text
	state    uint      // memo state: active, deleted, done
	priority int16     // memo order to display
	ts       time.Time // last op time
}

type RemindParams struct {
	RemindAt int    // remind time as number of minutes (hour * 60 + minute)
	TimeZone string // time zone identifier
	Set      bool   // remind or not
	ChatID   int64  // chat ID to send reminders to
}
