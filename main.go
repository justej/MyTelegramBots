package main

import (
	"telecho/database"
)

type State int

var (
	db     = database.NewDatabase()
	states = make(map[int64]State)
)

func main() {
	bot := InitBot()
	InitReminders(bot, &db)

	RunBot(bot)
}
