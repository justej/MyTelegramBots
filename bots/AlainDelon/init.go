package alaindelon

import (
	"botfarm/bot"
	"botfarm/bots/AlainDelon/db"
	"botfarm/bots/AlainDelon/tgbot"

	tg "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	_ "github.com/jackc/pgx/v5/stdlib"
	"go.uber.org/zap"
)

type AlainDelon struct{}

func (ad *AlainDelon) Init(cfg *bot.Config, l *zap.SugaredLogger) (*bot.Context, error) {
	d, err := db.Init(cfg.DBConnStr)
	if err != nil {
		l.Error("failed to initialize database")
		return nil, err
	}

	b, err := tg.NewBotAPI(cfg.TgToken)
	if err != nil {
		l.Error("failed to initialize Telegram Bot")
		return nil, err
	}

	b.Debug = false

	ctx := bot.Context{Bot: b, DB: d, Logger: l}
	return &ctx, nil
}

func (ad *AlainDelon) Run(ctx *bot.Context) {
	if ctx.Bot == nil {
		ctx.Logger.Error("can't run the bot because it's uninitialized")
		return
	}

	uCfg := tg.NewUpdate(0)
	uCfg.Timeout = 60

	for u := range ctx.Bot.GetUpdatesChan(uCfg) {
		if u.Message != nil {
			ctx := ctx.CloneWith(u.Message.From.ID)

			if u.Message.IsCommand() {
				go tgbot.HandleCommand(ctx, &u)
			} else {
				go tgbot.HandleUpdate(ctx, &u)
			}
		}

		if u.CallbackQuery != nil {
			ctx := ctx.CloneWith(u.CallbackQuery.From.ID)

			go tgbot.HandleCallbackQuery(ctx, &u)
		}
	}
}

func init() {
	bot.Register("AlainDebot", &AlainDelon{})
}
