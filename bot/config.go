package bot

// Mandatory parameters required to start the bot.
const (
	CfgTgToken         = "TgToken"
	CfgDbConnStr       = "DBConnStr"
	CfgDbRetryAttempts = "DBRetryAttempts"
	CfgDbRetryDelay    = "DBRetryDelay"
	CfgDbTimeout       = "DBTimeout"
)

// Config keeps bot configuration
type Config = map[string]any
