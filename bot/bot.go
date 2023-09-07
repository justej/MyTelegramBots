package bot

import (
	"sync"
)

// Each bot should implement the Bot interface.
type Bot interface {
	// Init method initializes the bot (connects to database, configures Telegram
	// Bot, etc.) and returns a context that should be used in the bot. On
	// failure, Init should return an error rather than panic.
	Init(Config) (Context, error)
	// Run starts the process of handling messages from the Telegram Bot. Multiple
	// bots are supposed to run concurrently, so Run should be started in a new
	// goroutine.
	Run(Context)
}

var (
	botsRegistry = make(map[string]Bot)
	botsMu       sync.Mutex
)

// Register adds the bot to the list of bots to run. To register a bot call
// Register in the init function.
func Register(name string, bot Bot) bool {
	botsMu.Lock()
	defer botsMu.Unlock()

	_, ok := botsRegistry[name]
	if ok {
		return false
	}

	botsRegistry[name] = bot
	return true
}

// Named bot record in the bots registry.
type Record struct {
	Name string
	Bot  *Bot
}

// GetThemAll returns sorted list of bots.
func GetThemAll() []Record {
	botsMu.Lock()
	defer botsMu.Unlock()

	bots := []Record{}
	for n, b := range botsRegistry {
		b := b
		bots = append(bots, Record{Name: n, Bot: &b})
	}

	return bots
}
