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
	Logger *zap.SugaredLogger
	values map[string]any
}

// Get fetches value for the given key.
// The operation is not thread-safe.
func (ctx *Context) Get(key string) (any, bool) {
	val, ok := ctx.values[key]
	if !ok {
		return nil, false
	}
	return val, true
}

// Put sets value for the given key. If the value already exists it will be
// overwritten silently.
// The operation is not thread-safe.
func (ctx *Context) Put(key string, val any) {
	ctx.values[key] = val
}

// Clone creates a copy of the context where Logger and Values are updated with "usr"=usr
func (ctx *Context) CloneWith(usr int64) *Context {
	newCtx := Context{
		Bot: ctx.Bot,
		DB: ctx.DB,
		Logger: ctx.Logger.With("usr", usr),
		values: make(map[string]any),
	}

	for k, v := range ctx.values {
		newCtx.Put(k, v)
	}
	newCtx.Put("usr", usr)

	return &newCtx
}