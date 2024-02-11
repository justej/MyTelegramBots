package alaindelon

import (
	"botfarm/bot"
	"botfarm/bots/AlainDelon/db"
	"botfarm/bots/AlainDelon/tgbot"
	"errors"

	tg "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	_ "github.com/jackc/pgx/v5/stdlib"
	"go.uber.org/zap"
)

type AlainDelon struct {
	ctx *bot.Context
}

func (ad *AlainDelon) Name() string {
	return "AlainDebot"
}

func (ad *AlainDelon) Init(cfg bot.Config, l *zap.SugaredLogger) error {
	v, ok := cfg[bot.CfgDbConnStr].(string)
	if !ok {
		l.Error("failed fetching connection string")
		return errors.New("configuration value doesn't exist")
	}
	d, err := db.Init(v)
	if err != nil {
		l.Errorw("failed to initialize database", "err", err)
		return err
	}

	v, ok = cfg[bot.CfgTgToken].(string)
	if !ok {
		l.Error("failed fetching Telegram token")
		return errors.New("configuration value doesn't exist")
	}
	b, err := tg.NewBotAPI(v)
	if err != nil {
		l.Errorw("failed to initialize Telegram Bot", "err", err)
		return err
	}

	b.Debug = false

	l.Infof("authorized on account %q", b.Self.UserName)

	ad.ctx = &bot.Context{Bot: b, DB: d, Logger: l}
	return nil
}

func (ad *AlainDelon) Run() {
	if ad.ctx.Bot == nil {
		ad.ctx.Logger.Error("can't run the bot because it's uninitialized")
		return
	}

	uCfg := tg.NewUpdate(0)
	uCfg.Timeout = 60

	for u := range ad.ctx.Bot.GetUpdatesChan(uCfg) {
		if u.Message != nil {
			ctx := ad.ctx.CloneWith(u.Message.From.ID)

			if u.Message.IsCommand() {
				go tgbot.HandleCommand(ctx, &u)
			} else {
				go tgbot.HandleUpdate(ctx, &u)
			}
		}

		if u.CallbackQuery != nil {
			ctx := ad.ctx.CloneWith(u.CallbackQuery.From.ID)

			go tgbot.HandleCallbackQuery(ctx, &u)
		}
	}
}

func init() {
	bot.Register(&AlainDelon{}, []string{bot.CfgDbConnStr, bot.CfgTgToken})
}
