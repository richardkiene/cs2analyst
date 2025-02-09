package main

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/richardkiene/cs2analyst/analyzer"
	"github.com/richardkiene/cs2analyst/collector"
	"github.com/spf13/cobra"
)

type config struct {
	demoPath   string
	playerName string
	mapsDir    string
	modelsDir  string
	logLevel   string
	output     string
	outputPath string
	dbConfig   string
}

func newRootCmd() *cobra.Command {
	cfg := &config{}

	cmd := &cobra.Command{
		Use:   "cs2democollector",
		Short: "Collect and analyze CS2 demo files",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateConfig(cfg); err != nil {
				return err
			}

			if err := run(cfg); err != nil {
				return err
			}

			return nil
		},
	}

	flags := cmd.Flags()
	flags.StringVar(&cfg.demoPath, "demo", "", "path to CS2 demo file (required)")
	flags.StringVar(&cfg.playerName, "player", "", "collect stats for specific player")
	flags.StringVar(&cfg.mapsDir, "maps", "", "path to directory containing map .obj files (required)")
	flags.StringVar(&cfg.modelsDir, "models", "", "path to directory containing player model .obj files (required)")
	flags.StringVar(&cfg.logLevel, "log-level", "info", "log level (debug, info, warn, error)")
	flags.StringVar(&cfg.output, "output", "", "output format (json, protobuf, db) (required)")
	flags.StringVar(&cfg.outputPath, "output-path", "", "path for json/protobuf output")
	flags.StringVar(&cfg.dbConfig, "db-config", "", "database configuration for db output")

	cmd.MarkFlagRequired("demo")
	cmd.MarkFlagRequired("maps")
	cmd.MarkFlagRequired("models")
	cmd.MarkFlagRequired("output")

	return cmd
}

func validateConfig(cfg *config) error {
	switch cfg.output {
	case "json", "protobuf":
		if cfg.outputPath == "" {
			return fmt.Errorf("output path required for json/protobuf output")
		}
	case "db":
		if cfg.dbConfig == "" {
			return fmt.Errorf("db config required for database output")
		}
	default:
		return fmt.Errorf("invalid output format: must be json, protobuf, or db")
	}

	return nil
}

func run(cfg *config) error {
	// Configure logging
	logLevel := new(slog.LevelVar)
	if err := logLevel.UnmarshalText([]byte(cfg.logLevel)); err != nil {
		return fmt.Errorf("invalid log level: %v", err)
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel}))
	slog.SetDefault(logger)

	// Initialize collector and analyzer
	c := collector.New()
	a, analyzerErr := analyzer.New()
	if analyzerErr != nil {
		return fmt.Errorf("failed to instantiate analyzer")
	}

	// Process demo file
	_, err := c.Collect(cfg.demoPath)
	if err != nil {
		return fmt.Errorf("failed to process demo: %w", err)
	}

	// Analyze collected data
	results, err := a.Analyze(c.PerTickInfo, c.TickRate, c.TickTime)
	if err != nil {
		return fmt.Errorf("failed to analyze data: %w", err)
	}

	// Output to console for now
	for steamID, medianTimeToDamage := range results {
		logger.Info("Time To Damage Calculated", "steamID", steamID, "TTD", medianTimeToDamage)
	}

	return nil
}

func main() {
	if err := newRootCmd().Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
