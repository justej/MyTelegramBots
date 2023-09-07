package main

import (
	"botfarm/bot"
	"encoding/json"
	"errors"
	"os"

	"go.uber.org/zap"
	_ "botfarm/bots/FindingMemo"
)

func getLogger() (*zap.SugaredLogger, func() error) {
	logger, _ := zap.NewDevelopment(zap.Fields(zap.String("ns", "Global")))

	log := logger.Sugar()
	return log, logger.Sync
}

func readConfig(cfgFile string) (map[string]bot.Config, error) {
	cfg, err := os.ReadFile(cfgFile)
	if err != nil {
		return nil, err
	}

	var farmConfig map[string]bot.Config
	err = json.Unmarshal(cfg, &farmConfig)
	if err != nil {
		return nil, errors.New("Couldn't unmarshal configuration")
	}

	return farmConfig, nil
}

func main() {
	log, syncLogs := getLogger()
	defer syncLogs()

	cfgFile, ok := os.LookupEnv("CONFIG_FILE")
	if !ok {
		log.Fatalf("Configuration file name is't set")
	}

	botConfigs, err := readConfig(cfgFile)
	if err != nil || botConfigs == nil {
		log.With(zap.Error(err)).Fatalf("Couldn't read configuration from file %q", cfgFile)
	}

	for _, rec := range bot.GetThemAll() {
		cfg := botConfigs[rec.Name]
		bot := *rec.Bot

		ctx, err := bot.Init(cfg)
		if err != nil {
			log.With(zap.Error(err)).Errorf("Failed to initialize bot '%s'", rec.Name)
			// we could've failed here but I prefer to keep running bots that can run
			continue
		}

		ctx.Logger, _ = zap.NewDevelopment(zap.Fields(zap.String("ns", rec.Name)))
		go bot.Run(ctx)
	}

	stuckHere := make(<-chan int)
	<-stuckHere
}
