package findingmemo

import (
	"botfarm/bot"
	"botfarm/bots/findingmemo/database"
	"botfarm/bots/findingmemo/reminder"
	"botfarm/bots/findingmemo/tgbot"
	"botfarm/bots/findingmemo/timezone"
)

type FindingMemo struct{}

func (b *FindingMemo) Init(cfg bot.Config) (bot.Context, error) {
	timezone.Init()
	db := database.Init(cfg.DBConnStr)
	tgbot.Init(cfg.TgToken, db)
	reminder.Init(db, tgbot.SendReminder)

	return bot.Context{}, nil
}

func (b *FindingMemo) Run(ctx bot.Context) {
	tgbot.Run()
}

func init() {
	bot.Register("FindingMemoBot", &FindingMemo{})
}
