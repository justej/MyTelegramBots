package database

import "time"

type user struct {
	userID    int64
	chatID    int64
	remind    bool
	remindAt  time.Time
	utcOffset int
	latitude  float32
	longitude float32
}

const (
	stateActive uint = iota
	stateDone
	stateDeleted
)

type memo struct {
	text     string
	state    uint // active, deleted, done
	priority int16
	ts       time.Time
}

type RemindParams struct {
	RemindAt  time.Time
	UTCOffset int
	Set       bool
	ChatID    int64
}
