package alaindelon

import (
	"botfarm/bot"
	"botfarm/bots/AlainDelon/db"
	"botfarm/bots/AlainDelon/tgbot"

	_ "github.com/jackc/pgx/v5/stdlib"
)

type AlainDelon struct{}

func (b *AlainDelon) Init(cfg bot.Config) (bot.Context, error) {
	db.Init(cfg.DBConnStr)
	tgbot.Init(cfg.TgToken)
	return bot.Context{}, nil
}

func (b *AlainDelon) Run(ctx bot.Context) {
	tgbot.Run()
}

func init() {
	bot.Register("AlainDebot", &AlainDelon{})
}
