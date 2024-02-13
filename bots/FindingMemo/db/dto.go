package db

import "time"

const (
	memoStateActive uint = iota
	memoStateDone
	memoStateDeleted
)

const (
	priorityMinValue = 1
)

type Memo struct {
	ID       int
	Text     string    // memo text
	State    uint      // memo state: active, deleted, done
	Priority int16     // memo order to display
	TS       time.Time // last op time
}

type RemindParams struct {
	RemindAt int    // remind time as number of minutes (hour * 60 + minute)
	TimeZone string // time zone identifier
	Set      bool   // remind or not
	ChatID   int64  // chat ID to send reminders to
}
