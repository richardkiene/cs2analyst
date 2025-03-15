package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"runtime/trace"

	"github.com/richardkiene/cs2analyst/analyzer"
	"github.com/richardkiene/cs2analyst/collector"
	"github.com/richardkiene/cs2analyst/trainer"
	"github.com/spf13/cobra"
)

type config struct {
	// Existing fields
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

	// Training-specific fields
	isTraining      bool
	baseModel       string
	modelPath       string
	batchSize       int
	epochs          int
	learningRate    float64
	validationSplit float64
}

func newRootCmd() *cobra.Command {
	cfg := &config{}

	rootCmd := &cobra.Command{
		Use:   "cs2analyst",
		Short: "Collect and analyze CS2 demo files",
	}

	// Add analyze command
	analyzeCmd := &cobra.Command{
		Use:   "analyze",
		Short: "Analyze CS2 demo files",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateAnalyzeConfig(cfg); err != nil {
				return err
			}
			return runAnalyze(cfg)
		},
	}

	// Add train command
	trainCmd := &cobra.Command{
		Use:   "train",
		Short: "Train a model using CS2 demo files",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg.isTraining = true
			if err := validateTrainingConfig(cfg); err != nil {
				return err
			}
			return runTraining(cfg)
		},
	}

	// Add flags to analyze command
	analyzeFlags := analyzeCmd.Flags()
	analyzeFlags.StringVar(&cfg.demoPath, "demo", "", "path to CS2 demo file (required)")
	analyzeFlags.StringVar(&cfg.playerName, "player", "", "collect stats for specific player")
	analyzeFlags.StringVar(&cfg.mapsDir, "maps", "", "path to directory containing map .obj files (required)")
	analyzeFlags.StringVar(&cfg.modelsDir, "models", "", "path to directory containing player model .obj files (required)")
	analyzeFlags.StringVar(&cfg.logLevel, "log-level", "info", "log level (debug, info, warn, error)")
	analyzeFlags.StringVar(&cfg.output, "output", "", "output format (json, protobuf, db) (required)")
	analyzeFlags.StringVar(&cfg.outputPath, "output-path", "", "path for json/protobuf output")
	analyzeFlags.StringVar(&cfg.dbConfig, "db-config", "", "database configuration for db output")
	analyzeFlags.IntVar(&cfg.debugTick, "debug-tick", -1, "generate debug visualization for specific tick")
	analyzeFlags.Uint64Var(&cfg.debugShooterID, "debug-shooter", 0, "steamID of the shooter for debug visualization")
	analyzeFlags.Uint64Var(&cfg.debugTargetID, "debug-target", 0, "steamID of the target for debug visualization")

	// Add flags to train command
	trainFlags := trainCmd.Flags()
	trainFlags.StringVar(&cfg.demoPath, "demo", "", "path to CS2 demo file for training (required)")
	trainFlags.StringVar(&cfg.mapsDir, "maps", "", "path to directory containing map .obj files (required)")
	trainFlags.StringVar(&cfg.modelsDir, "models", "", "path to directory containing player model .obj files (required)")
	trainFlags.StringVar(&cfg.baseModel, "base-model", "codellama", "base model to use for training")
	trainFlags.StringVar(&cfg.modelPath, "model-path", "", "path to save the trained model (required)")
	trainFlags.IntVar(&cfg.batchSize, "batch-size", 32, "training batch size")
	trainFlags.IntVar(&cfg.epochs, "epochs", 100, "number of training epochs")
	trainFlags.Float64Var(&cfg.learningRate, "learning-rate", 0.001, "learning rate for training")
	trainFlags.Float64Var(&cfg.validationSplit, "validation-split", 0.2, "portion of data to use for validation")
	trainFlags.StringVar(&cfg.logLevel, "log-level", "info", "log level (debug, info, warn, error)")

	// Add commands to root
	rootCmd.AddCommand(analyzeCmd)
	rootCmd.AddCommand(trainCmd)

	return rootCmd
}

func validateAnalyzeConfig(cfg *config) error {
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

	if cfg.debugTick >= 0 || cfg.debugShooterID != 0 || cfg.debugTargetID != 0 {
		if cfg.debugTick < 0 || cfg.debugShooterID == 0 || cfg.debugTargetID == 0 {
			return fmt.Errorf("debug-tick, debug-shooter, and debug-target must all be specified together")
		}
	}

	return nil
}

func validateTrainingConfig(cfg *config) error {
	if cfg.demoPath == "" {
		return fmt.Errorf("demo path is required")
	}
	if cfg.mapsDir == "" {
		return fmt.Errorf("maps directory is required")
	}
	if cfg.modelsDir == "" {
		return fmt.Errorf("models directory is required")
	}
	if cfg.modelPath == "" {
		return fmt.Errorf("model path is required")
	}
	if cfg.batchSize <= 0 {
		return fmt.Errorf("batch size must be positive")
	}
	if cfg.epochs <= 0 {
		return fmt.Errorf("epochs must be positive")
	}
	if cfg.learningRate <= 0 {
		return fmt.Errorf("learning rate must be positive")
	}
	if cfg.validationSplit <= 0 || cfg.validationSplit >= 1 {
		return fmt.Errorf("validation split must be between 0 and 1")
	}
	return nil
}

func runAnalyze(cfg *config) error {
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
	// TODO: When match is actually populated, use it.
	_, err := c.Collect(cfg.demoPath)
	if err != nil {
		return fmt.Errorf("failed to process demo: %w", err)
	}

	// Analyze collected data
	results, err := a.Analyze(c.PerTickInfo, c.ActiveSmokes, c.TickRate, c.TickTime)
	if err != nil {
		return fmt.Errorf("failed to analyze data: %w", err)
	}

	for steamID, medianTimeToDamage := range results {
		logger.Info("Time To Damage Calculated", "steamID", steamID, "TTD", medianTimeToDamage)
	}

	return nil
}

func runTraining(cfg *config) error {
	// Configure logging
	logLevel := new(slog.LevelVar)
	if err := logLevel.UnmarshalText([]byte(cfg.logLevel)); err != nil {
		return fmt.Errorf("invalid log level: %v", err)
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel}))
	slog.SetDefault(logger)

	// Create training configuration
	trainingConfig := trainer.TrainingConfig{
		BaseModel:       cfg.baseModel,
		ModelPath:       cfg.modelPath,
		BatchSize:       cfg.batchSize,
		Epochs:          cfg.epochs,
		LearningRate:    cfg.learningRate,
		ValidationSplit: cfg.validationSplit,
		DemoPath:        cfg.demoPath,
		MapObjPath:      cfg.mapsDir,
		PlayerObjPath:   cfg.modelsDir,
	}

	// Initialize trainer
	t, err := trainer.NewTrainer(trainingConfig, logger)
	if err != nil {
		return fmt.Errorf("failed to initialize trainer: %w", err)
	}

	// Load models
	if err := t.LoadModels(cfg.mapsDir, cfg.modelsDir); err != nil {
		return fmt.Errorf("failed to load models: %w", err)
	}

	// Initialize collector
	c := collector.New()

	// Collect demo data
	logger.Info("Collecting demo data for training", "demo_path", cfg.demoPath)
	match, err := c.Collect(cfg.demoPath)
	if err != nil {
		return fmt.Errorf("failed to collect demo data: %w", err)
	}

	// Convert collector data into training examples
	examples, err := t.ConvertCollectorData(c.PerTickInfo, match)
	if err != nil {
		return fmt.Errorf("failed to convert training data: %w", err)
	}

	// Start training
	logger.Info("Starting model training",
		"num_examples", len(examples),
		"epochs", cfg.epochs,
		"batch_size", cfg.batchSize)

	ctx := context.Background()
	if err := t.Train(ctx, examples); err != nil {
		return fmt.Errorf("training failed: %w", err)
	}

	// Get and log final metrics
	metrics := t.GetMetrics()
	if len(metrics) > 0 {
		final := metrics[len(metrics)-1]
		logger.Info("Training completed",
			"epochs_completed", final.Epoch,
			"final_training_loss", final.TrainingLoss,
			"final_validation_loss", final.ValidationLoss,
			"feedback_accuracy", final.FeedbackAccuracy,
			"tactical_precision", final.TacticalPrecision,
			"positional_accuracy", final.PositionalAccuracy,
			"total_duration", final.TrainingDuration)
	}

	return nil
}

func main() {
	if err := newRootCmd().Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
