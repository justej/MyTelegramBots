package bot

import (
	"database/sql"

	tg "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"go.uber.org/zap"
)

// Bot context keeps references to common (Telegram Bot API, database, logger)
// and individual parameters of a bot.
type Context struct {
	Bot    *tg.BotAPI
	DB     *sql.DB
	Logger *zap.Logger
	Values map[string]any
}

// NewContext creates new context. Make sure pointers are not nil.
func NewContext(bot *tg.BotAPI, db *sql.DB, logger *zap.Logger) Context {
	return Context{
		Bot:    bot,
		DB:     db,
		Logger: logger,
		Values: make(map[string]any),
	}
}

// Get fetches value for the given key.
// The operation is not thread-safe.
func (ctx *Context) Get(key string) (any, bool) {
	val, ok := ctx.Values[key]
	if !ok {
		return nil, false
	}
	return val, true
}

// Put sets value for the given key. If the value already exists it will be
// overwritten silently.
// The operation is not thread-safe.
func (ctx *Context) Put(key string, val *any) {
	ctx.Values[key] = val
}
