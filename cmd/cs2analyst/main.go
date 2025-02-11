package main

import (
	"fmt"
	"log/slog"
	"os"
	"runtime/trace"

	"github.com/richardkiene/cs2analyst/analyzer"
	"github.com/richardkiene/cs2analyst/collector"
	"github.com/spf13/cobra"
)

type config struct {
	demoPath       string
	playerName     string
	mapsDir        string
	modelsDir      string
	logLevel       string
	output         string
	outputPath     string
	dbConfig       string
	debugTick      int
	debugShooterID uint64
	debugTargetID  uint64
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
	flags.IntVar(&cfg.debugTick, "debug-tick", -1, "generate debug visualization for specific tick")
	flags.Uint64Var(&cfg.debugShooterID, "debug-shooter", 0, "steamID of the shooter for debug visualization")
	flags.Uint64Var(&cfg.debugTargetID, "debug-target", 0, "steamID of the target for debug visualization")

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

	// Debug flags validation
	if cfg.debugTick >= 0 || cfg.debugShooterID != 0 || cfg.debugTargetID != 0 {
		if cfg.debugTick < 0 || cfg.debugShooterID == 0 || cfg.debugTargetID == 0 {
			return fmt.Errorf("debug-tick, debug-shooter, and debug-target must all be specified together")
		}
	}

	return nil
}

func run(cfg *config) error {
	// Configure logging
	logLevel := new(slog.LevelVar)
	if err := logLevel.UnmarshalText([]byte(cfg.logLevel)); err != nil {
		return fmt.Errorf("invalid log level: %v", err)
	}

	if logLevel.Level() == slog.LevelDebug {
		f, err := os.Create("trace.out")
		if err != nil {
			panic(err)
		}
		defer f.Close()
		if err := trace.Start(f); err != nil {
			panic(err)
		}
		defer trace.Stop()
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

	// If debug flags are set, generate debug visualization
	if cfg.debugTick >= 0 && cfg.debugShooterID != 0 && cfg.debugTargetID != 0 {
		logger.Info("Generating debug visualization",
			"tick", cfg.debugTick,
			"shooterID", cfg.debugShooterID,
			"targetID", cfg.debugTargetID)

		if err := a.GenerateDebugVisualization(
			cfg.debugTick,
			cfg.debugShooterID,
			cfg.debugTargetID,
			c.PerTickInfo,
		); err != nil {
			return fmt.Errorf("failed to generate debug visualization: %w", err)
		}
		return nil
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
