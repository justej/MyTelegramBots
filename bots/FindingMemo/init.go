package findingmemo

import (
	"botfarm/bot"
	"botfarm/bots/FindingMemo/db"
	"botfarm/bots/FindingMemo/reminder"
	"botfarm/bots/FindingMemo/tgbot"
	"botfarm/bots/FindingMemo/timezone"
	"errors"

	tg "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"go.uber.org/zap"
)

type FindingMemo struct {
	*tgbot.TBot
}

func requiredConfigFields() []string {
	return []string{
		bot.CfgDbConnStr,
		bot.CfgTgToken,
	}
}

func (fm *FindingMemo) Name() string {
	return "FindingMemo"
}

func (fm *FindingMemo) Init(cfg bot.Config, l *zap.SugaredLogger) error {
	// Time zone
	err := timezone.Init()
	if err != nil {
		l.Errorw("failed to initialize time zones", "err", err)
		return err
	}

	// Database
	v, ok := cfg[bot.CfgDbConnStr].(string)
	if !ok {
		l.Error("failed fetching connection string")
		return errors.New("configuration value doesn't exist")
	}
	d, err := db.NewDatabase(v)
	if err != nil {
		l.Errorw("failed to initialize database", "err", err)
		return err
	}

	// TBot
	v, ok = cfg[bot.CfgTgToken].(string)
	if !ok {
		l.Error("failed fetching Telegram token")
		return errors.New("configuration value doesn't exist")
	}

	fm.TBot, err = tgbot.NewTBot(v, d, l)
	if err != nil {
		return errors.New("failed creating TBot")
	}

	// Reminder
	rm := reminder.NewManager(d, fm.TBot.SendReminder, l)
	fm.TBot.ReminderManager = rm

	return nil
}

func (fm *FindingMemo) Run() {
	// Run reminder
	fm.TBot.ReminderManager.Run()

	// Run bot
	uCfg := tg.NewUpdate(0)
	uCfg.Timeout = 60

	for u := range fm.TBot.Bot.GetUpdatesChan(uCfg) {
		if u.Message != nil {
			if u.Message.IsCommand() {
				go fm.TBot.HandleCommand(u.Message)
			} else {
				go fm.TBot.HandleMessage(u.Message)
			}
		}
	}
}

func init() {
	bot.Register(&FindingMemo{}, requiredConfigFields())
}
