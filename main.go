package main

import (
	"telecho/database"
)

type State int

var states = make(map[int64]State)

func main() {
	db := database.InitDatabase()

	bot := InitBot()
	InitReminders(bot, db)

	RunBot(bot, db)
}
