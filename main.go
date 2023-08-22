package main

import (
	"findingmemo/database"
	"findingmemo/reminder"
	"findingmemo/tgbot"
	"findingmemo/timezone"
)

func main() {
	timezone.Init()
	db := database.Init()
	tgbot.Init(db)
	reminder.Init(db, tgbot.SendReminder)

	tgbot.Run()
}
