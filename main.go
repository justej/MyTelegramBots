package main

import (
	"telecho/database"
	"telecho/reminder"
	"telecho/tgbot"
	"telecho/timezone"
)

func main() {
	timezone.Init()
	db := database.Init()
	tgBot.Init(db)
	reminder.Init(db, tgBot.SendReminder)

	tgBot.Run()
}
