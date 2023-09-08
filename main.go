package main

import (
	"botfarm/bot"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	_ "botfarm/bots/AlainDelon"
	_ "botfarm/bots/FindingMemo"

	"go.uber.org/zap"
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
	logger, syncLogs := getLogger()
	defer syncLogs()

	cfgFile, ok := os.LookupEnv("CONFIG_FILE")
	if !ok {
		logger.Fatalf("Configuration file name is't set")
	}

	botConfigs, err := readConfig(cfgFile)
	if err != nil || botConfigs == nil {
		logger.Fatalw(fmt.Sprintf("Couldn't read configuration from file %q", cfgFile), "err", err)
	}

	for _, rec := range bot.GetThemAll() {
		cfg := botConfigs[rec.Name]
		b := *rec.Bot
		l, _ := zap.NewDevelopment(zap.Fields(zap.String("ns", rec.Name)))
		defer l.Sync()

		ctx, err := b.Init(&cfg, l.Sugar())
		if err != nil {
			// we could've failed here but I prefer to keep running bots that can run
			continue
		}

		ctx.Logger.Infof("successfully initialized bot as %q", ctx.Bot.Self.UserName)

		go b.Run(ctx)
	}

	stuckHere := make(<-chan int)
	<-stuckHere
}
