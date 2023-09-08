package findingmemo

import (
	"botfarm/bot"
	"botfarm/bots/FindingMemo/db"
	"botfarm/bots/FindingMemo/reminder"
	"botfarm/bots/FindingMemo/tgbot"
	"botfarm/bots/FindingMemo/timezone"

	tg "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"go.uber.org/zap"
)

type FindingMemo struct{}

func (fm *FindingMemo) Init(cfg *bot.Config, l *zap.SugaredLogger) (*bot.Context, error) {
	err := timezone.Init()
	if err != nil {
		l.Errorw("failed to initialize time zones", "err", err)
		return nil, err
	}

	d, err := db.Init(cfg.DBConnStr)
	if err != nil {
		l.Errorw("failed to initialize database", "err", err)
		return nil, err
	}

	b, err := tg.NewBotAPI(cfg.TgToken)
	if err != nil {
		l.Error("failed to initialize Telegram Bot")
		return nil, err
	}

	b.Debug = false

	l.Infof("authorized on account %q", b.Self.UserName)

	ctx := &bot.Context{Bot: b, DB: d, Logger: l}

	reminder.Init(ctx, tgbot.SendReminder)

	return ctx, nil
}

func (fm *FindingMemo) Run(ctx *bot.Context) {
	if ctx.Bot == nil {
		ctx.Logger.Warn("Bot can't run")
		return
	}

	uCfg := tg.NewUpdate(0)
	uCfg.Timeout = 60

	for u := range ctx.Bot.GetUpdatesChan(uCfg) {
		if u.Message != nil {
			ctx := ctx.CloneWith(u.Message.From.ID)
			if u.Message.IsCommand() {
				go tgbot.HandleCommand(ctx, u.Message)
			} else {
				go tgbot.HandleMessage(ctx, u.Message)
			}
		}
	}
}

func init() {
	bot.Register("FindingMemoBot", &FindingMemo{})
}
