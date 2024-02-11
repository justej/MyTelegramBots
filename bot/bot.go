package bot

import (
	"sync"

	"go.uber.org/zap"
)

// Bot is an interface each bot should implement
type Bot interface {
	// Name returns the name of the bot
	Name() string
	// Init method initializes the bot (connects to database, configures Telegram
	// Bot, etc.). On failure, Init should log the error and return it rather than
	// panic.
	Init(Config, *zap.SugaredLogger) error
	// Run starts the process of handling messages from the Telegram Bot. Multiple
	// bots are supposed to run concurrently, so Run should be started in a new
	// goroutine.
	Run()
}

var (
	botsRegistry = make(map[string]Record)
	botsMu       sync.Mutex
)

// Register adds the bot to the list of bots to run. To register a bot call
// Register in the init function.
func Register(bot Bot, rcf []string) bool {
	botsMu.Lock()
	defer botsMu.Unlock()

	name := bot.Name()
	_, ok := botsRegistry[name]
	if ok {
		return false
	}

	botsRegistry[name] = Record{
		Name:                 name,
		Bot:                  &bot,
		RequiredConfigFields: rcf,
	}

	return true
}

// Record represents named bot record in the bots registry. It also contains
// fields to verify bot configuration.
type Record struct {
	Name                 string
	Bot                  *Bot
	RequiredConfigFields []string
}

// GetThemAll returns list of bots.
func GetThemAll() []Record {
	botsMu.Lock()
	defer botsMu.Unlock()

	bots := []Record{}
	for _, r := range botsRegistry {
		bots = append(bots, r)
	}

	return bots
}
