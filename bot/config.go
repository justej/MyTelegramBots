package bot

// Mandatory parameters required to start the bot.
const (
	CfgTgToken                  = "TgToken"
	CfgDbConnStr                = "DBConnStr"
)

// Config keeps bot configuration
type Config = map[string]any
