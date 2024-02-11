package main

import (
	"botfarm/bot"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	_ "botfarm/bots/AlainDelon"
	_ "botfarm/bots/FindingMemo"

	"go.uber.org/zap"
)

const stopOnFailure = false

// getLogger creates a logger in global namespace
func getLogger() (*zap.SugaredLogger, func() error) {
	logger, _ := zap.NewDevelopment(zap.Fields(zap.String("ns", "Global")))

	log := logger.Sugar()
	return log, logger.Sync
}

// readConfig reads configuration from the given file
func readConfig(cfgFile string) (map[string]any, error) {
	cfg, err := os.ReadFile(cfgFile)
	if err != nil {
		return nil, err
	}

	var botfarmConfig map[string]any
	err = json.Unmarshal(cfg, &botfarmConfig)
	if err != nil {
		return nil, errors.New("Couldn't unmarshal botfarm configuration")
	}

	return botfarmConfig, nil
}

// validateConfig makes sure that all required fields are present in the config
func validateConfig(rec bot.Record, cfg bot.Config) error {
	missingFields := []string{}
	for _, field := range rec.RequiredConfigFields {
		if _, ok := cfg[field]; !ok {
			missingFields = append(missingFields, field)
		}
	}

	if len(missingFields) > 0 {
		return errors.New(fmt.Sprintf("%v's configuration is missing field(s): %s", rec.Name, strings.Join(missingFields, ", ")))
	}

	return nil
}

// Botfarm entry point
func main() {
	logger, syncLogs := getLogger()
	defer syncLogs()

	cfgFile, ok := os.LookupEnv("CONFIG_FILE")
	if !ok {
		logger.Fatalf("Configuration file name isn't set")
	}

	botConfigs, err := readConfig(cfgFile)
	if err != nil || botConfigs == nil {
		logger.Fatalw(fmt.Sprintf("Couldn't read configuration from file %q", cfgFile), "err", err)
	}

	for _, rec := range bot.GetThemAll() {
		b := *rec.Bot
		l, _ := zap.NewDevelopment(zap.Fields(zap.String("ns", rec.Name)))
		s := l.Sugar()
		defer l.Sync()

		c, ok := botConfigs[rec.Name]
		if !ok {
			s.Errorf("Couldn't find configuration for bot %q", rec.Name)
			if stopOnFailure {
				return
			} else {
				continue
			}
		}

		cfg, ok := c.(bot.Config)
		if !ok {
			s.Errorf("Couldn't find configuration for bot %q", rec.Name)
			if stopOnFailure {
				return
			} else {
				continue
			}
		}

		err := validateConfig(rec, cfg)
		if err != nil {
			s.Error(err)
			if stopOnFailure {
				return
			} else {
				continue
			}
		}

		err = b.Init(cfg, s)
		if err != nil {
			if stopOnFailure {
				return
			} else {
				continue
			}
		}

		go b.Run()
	}

	stickHere := make(<-chan int)
	<-stickHere
}
