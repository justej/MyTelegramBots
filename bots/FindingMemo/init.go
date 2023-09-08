package findingmemo

import (
	"botfarm/bot"
	"botfarm/bots/findingmemo/database"
	"botfarm/bots/findingmemo/reminder"
	"botfarm/bots/findingmemo/tgbot"
	"botfarm/bots/findingmemo/timezone"

	"go.uber.org/zap"
)

type FindingMemo struct{}

func (fm *FindingMemo) Init(cfg *bot.Config, l *zap.SugaredLogger) (*bot.Context, error) {
	timezone.Init()
	db := database.Init(cfg.DBConnStr)
	err := tgbot.Init(cfg.TgToken, db)
	if err != nil {
		return nil, err
	}
	reminder.Init(db, tgbot.SendReminder)

	return &bot.Context{}, nil
}

func (fm *FindingMemo) Run(ctx *bot.Context) {
	tgbot.Run()
}

func init() {
	bot.Register("FindingMemoBot", &FindingMemo{})
}
