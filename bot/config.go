package bot

// Parameters required to start the bot.
type Config struct {
	TgToken     string `json:"tgtoken"`
	DBConnStr string `json:"dbconnstr"`
}