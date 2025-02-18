package trainer

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/golang/geo/r3"
	"github.com/richardkiene/cs2analyst/types"
	"github.com/richardkiene/cs2analyst/visibility"
)

// TrainingConfig holds the configuration for model training
type TrainingConfig struct {
	BaseModel        string             // Name of base model to use (e.g., "codellama")
	ModelPath        string             // Path to save trained model
	BatchSize        int                // Training batch size
	Epochs           int                // Number of training epochs
	LearningRate     float64            // Learning rate for training
	ValidationSplit  float64            // Portion of data to use for validation
	MapFeatures      map[string]string  // Map callouts and features
	ProPlayerMetrics map[string]float64 // Reference metrics from pro players
	DemoPath         string             // Path to demo file for training
	MapObjPath       string             // Path to map .obj file
	PlayerObjPath    string             // Path to player model .obj file
}

// TrainingMetrics tracks the model's performance during training
type TrainingMetrics struct {
	Epoch              int
	TrainingLoss       float64
	ValidationLoss     float64
	FeedbackAccuracy   float64
	TacticalPrecision  float64
	PositionalAccuracy float64
	EconomicPrecision  float64
	ComparisonAccuracy float64
	TrainingDuration   time.Duration
	TimestampUTC       time.Time
}

// Trainer handles the model training process
type Trainer struct {
	config      TrainingConfig
	logger      *slog.Logger
	model       *Model
	mapModel    *visibility.Model
	playerModel *visibility.Model
	metrics     []TrainingMetrics
	mutex       sync.RWMutex
}

// Model represents the Ollama model being trained
type Model struct {
	Name       string
	Parameters map[string]interface{}
	Path       string
}

// NewTrainer creates a new trainer instance
func NewTrainer(config TrainingConfig, logger *slog.Logger) (*Trainer, error) {
	if logger == nil {
		logger = slog.New(slog.NewJSONHandler(os.Stdout, nil))
	}

	t := &Trainer{
		config:  config,
		logger:  logger,
		metrics: make([]TrainingMetrics, 0),
	}

	// Initialize the model
	model, err := t.initializeModel()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize model: %w", err)
	}
	t.model = model

	return t, nil
}

// initializeModel sets up the base model for training
func (t *Trainer) initializeModel() (*Model, error) {
	t.logger.Info("Initializing model", "base_model", t.config.BaseModel)

	// Create model with initial parameters
	model := &Model{
		Name: t.config.BaseModel,
		Parameters: map[string]interface{}{
			"temperature":    0.7,
			"top_p":          0.9,
			"context_length": 4096,
		},
		Path: t.config.ModelPath,
	}

	// TODO: Add Ollama-specific initialization
	// This will involve using the Ollama API to create/initialize the model

	return model, nil
}

// LoadModels loads the map and player models
func (t *Trainer) LoadModels(mapObjPath, playerObjPath string) error {
	var err error

	t.mapModel, err = visibility.LoadOBJ(mapObjPath)
	if err != nil {
		return fmt.Errorf("failed to load map model: %w", err)
	}

	t.playerModel, err = visibility.LoadOBJ(playerObjPath)
	if err != nil {
		return fmt.Errorf("failed to load player model: %w", err)
	}

	return nil
}

// TrainingExample represents a single training instance
type TrainingExample struct {
	TimeSeriesData   map[int]map[uint64]types.PlayerTickData
	MapState         MapState
	ExpectedFeedback []string
	PlayerPositions  []PlayerPosition
	RoundEconomy     EconomyState
	UtilityUsage     []UtilityEvent
	CombatEvents     []CombatEvent
}

// Supporting types for TrainingExample
type MapState struct {
	AreaName     string
	Angles       []string
	CoverPoints  []r3.Vector
	ClearPoints  []r3.Vector
	Obstructions []r3.Vector
}

type PlayerPosition struct {
	Tick            int
	Position        r3.Vector
	ViewAngles      r3.Vector
	CrosshairHeight float64
	VisibleAreas    []string
}

type EconomyState struct {
	Round        int
	TeamMoney    int
	PlayerMoney  int
	EnemyEconomy string
	BuyDecision  string
	OptimalBuy   string
}

type UtilityEvent struct {
	Type      string
	Position  r3.Vector
	Impact    float64
	Timestamp int
}

type CombatEvent struct {
	Tick               int
	AttackerID         uint64
	VictimID           uint64
	WeaponType         string
	DamageDealt        int
	IsHeadshot         bool
	TimeToDamage       float64
	CrosshairPlacement float64
}

// Train trains the model using the provided time series data
func (t *Trainer) Train(ctx context.Context, examples []TrainingExample) error {
	t.logger.Info("Starting training process",
		"num_examples", len(examples),
		"epochs", t.config.Epochs,
		"batch_size", t.config.BatchSize)

	// Split data into training and validation sets
	splitIndex := int(float64(len(examples)) * (1 - t.config.ValidationSplit))
	trainingData := examples[:splitIndex]
	validationData := examples[splitIndex:]

	for epoch := 0; epoch < t.config.Epochs; epoch++ {
		epochStart := time.Now()

		// Training phase
		trainingLoss := t.trainEpoch(ctx, trainingData)

		// Validation phase
		validationMetrics := t.validate(ctx, validationData)

		// Record metrics
		metrics := TrainingMetrics{
			Epoch:              epoch,
			TrainingLoss:       trainingLoss,
			ValidationLoss:     validationMetrics.ValidationLoss,
			FeedbackAccuracy:   validationMetrics.FeedbackAccuracy,
			TacticalPrecision:  validationMetrics.TacticalPrecision,
			PositionalAccuracy: validationMetrics.PositionalAccuracy,
			EconomicPrecision:  validationMetrics.EconomicPrecision,
			ComparisonAccuracy: validationMetrics.ComparisonAccuracy,
			TrainingDuration:   time.Since(epochStart),
			TimestampUTC:       time.Now().UTC(),
		}

		t.recordMetrics(metrics)

		t.logger.Info("Completed epoch",
			"epoch", epoch,
			"training_loss", trainingLoss,
			"validation_loss", validationMetrics.ValidationLoss,
			"duration", metrics.TrainingDuration)

		// Check for early stopping
		if t.shouldEarlyStop() {
			t.logger.Info("Early stopping triggered")
			break
		}
	}

	return t.saveModel()
}

// trainEpoch trains the model for one epoch
func (t *Trainer) trainEpoch(ctx context.Context, examples []TrainingExample) float64 {
	var totalLoss float64

	for i := 0; i < len(examples); i += t.config.BatchSize {
		end := min(i+t.config.BatchSize, len(examples))
		batch := examples[i:end]

		batchLoss := t.trainBatch(ctx, batch)
		totalLoss += batchLoss

		if i%100 == 0 {
			t.logger.Debug("Batch training progress",
				"batch", i/t.config.BatchSize,
				"loss", batchLoss)
		}
	}

	return totalLoss / float64(len(examples))
}

// trainBatch trains on a single batch of examples
func (t *Trainer) trainBatch(ctx context.Context, batch []TrainingExample) float64 {
	// TODO: Implement actual Ollama training logic
	// This will involve:
	// 1. Converting batch data to the format expected by Ollama
	// 2. Making API calls to train the model
	// 3. Processing and returning the loss

	return 0.0 // Placeholder
}

// validate performs validation and returns metrics
func (t *Trainer) validate(ctx context.Context, examples []TrainingExample) TrainingMetrics {
	// TODO: Implement validation logic
	return TrainingMetrics{}
}

// recordMetrics records training metrics
func (t *Trainer) recordMetrics(metrics TrainingMetrics) {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	t.metrics = append(t.metrics, metrics)

	// Save metrics to disk
	if err := t.saveMetrics(); err != nil {
		t.logger.Error("Failed to save metrics", "error", err)
	}
}

// saveMetrics saves the training metrics to disk
func (t *Trainer) saveMetrics() error {
	metricsPath := filepath.Join(filepath.Dir(t.config.ModelPath), "training_metrics.json")

	data, err := json.MarshalIndent(t.metrics, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal metrics: %w", err)
	}

	if err := os.WriteFile(metricsPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write metrics file: %w", err)
	}

	return nil
}

// shouldEarlyStop determines if training should stop early
func (t *Trainer) shouldEarlyStop() bool {
	if len(t.metrics) < 3 {
		return false
	}

	// Check if validation loss hasn't improved for 3 epochs
	recent := t.metrics[len(t.metrics)-3:]
	return recent[2].ValidationLoss >= recent[1].ValidationLoss &&
		recent[1].ValidationLoss >= recent[0].ValidationLoss
}

// saveModel saves the trained model
func (t *Trainer) saveModel() error {
	// TODO: Implement Ollama model saving
	// This will involve making API calls to save the model state

	t.logger.Info("Model saved", "path", t.config.ModelPath)
	return nil
}

// GetMetrics returns the current training metrics
// DefaultConfig returns a default training configuration
func DefaultConfig() TrainingConfig {
	return TrainingConfig{
		BaseModel:        "codellama",
		BatchSize:        32,
		Epochs:           100,
		LearningRate:     0.001,
		ValidationSplit:  0.2,
		MapFeatures:      make(map[string]string),
		ProPlayerMetrics: make(map[string]float64),
	}
}

// ValidateConfig checks if the training configuration is valid
func ValidateConfig(cfg TrainingConfig) error {
	if cfg.DemoPath == "" {
		return fmt.Errorf("demo path is required")
	}
	if cfg.MapObjPath == "" {
		return fmt.Errorf("map obj path is required")
	}
	if cfg.PlayerObjPath == "" {
		return fmt.Errorf("player obj path is required")
	}
	if cfg.ModelPath == "" {
		return fmt.Errorf("model path is required")
	}
	if cfg.BatchSize <= 0 {
		return fmt.Errorf("batch size must be positive")
	}
	if cfg.Epochs <= 0 {
		return fmt.Errorf("epochs must be positive")
	}
	if cfg.LearningRate <= 0 {
		return fmt.Errorf("learning rate must be positive")
	}
	if cfg.ValidationSplit <= 0 || cfg.ValidationSplit >= 1 {
		return fmt.Errorf("validation split must be between 0 and 1")
	}
	return nil
}

func (t *Trainer) GetMetrics() []TrainingMetrics {
	t.mutex.RLock()
	defer t.mutex.RUnlock()

	metrics := make([]TrainingMetrics, len(t.metrics))
	copy(metrics, t.metrics)
	return metrics
}

// sequence represents a meaningful sequence of ticks (e.g., a round or engagement)
type sequence struct {
	startTick int
	endTick   int
	players   map[uint64]bool
}

// ConvertCollectorData converts raw collector data into structured training examples
func (t *Trainer) ConvertCollectorData(tickData map[int]map[uint64]types.PlayerTickData, match *types.Match) ([]TrainingExample, error) {
	t.logger.Info("Converting collector data to training examples",
		"tick_count", len(tickData),
		"player_count", len(match.PlayerStats))

	var examples []TrainingExample

	// Group ticks into rounds or significant sequences
	sequences := t.groupTicksIntoSequences(tickData)

	for _, seq := range sequences {
		example := TrainingExample{
			TimeSeriesData:  make(map[int]map[uint64]types.PlayerTickData),
			MapState:        t.buildMapState(seq),
			PlayerPositions: t.extractPlayerPositions(seq),
			RoundEconomy:    t.analyzeEconomy(seq, match),
			UtilityUsage:    t.extractUtilityUsage(seq),
			CombatEvents:    t.extractCombatEvents(seq),
		}

		// Copy relevant tick data
		for tick := seq.startTick; tick <= seq.endTick; tick++ {
			if playerData, exists := tickData[tick]; exists {
				example.TimeSeriesData[tick] = playerData
			}
		}

		examples = append(examples, example)
	}

	t.logger.Info("Conversion completed", "example_count", len(examples))
	return examples, nil
}

// groupTicksIntoSequences groups ticks into meaningful sequences for training
func (t *Trainer) groupTicksIntoSequences(tickData map[int]map[uint64]types.PlayerTickData) []sequence {
	var sequences []sequence

	// Find meaningful sequences based on game events
	var currentSeq *sequence
	const minSequenceLength = 100 // Minimum ticks for a sequence

	// Sort ticks for deterministic processing
	ticks := make([]int, 0, len(tickData))
	for tick := range tickData {
		ticks = append(ticks, tick)
	}
	sort.Ints(ticks)

	for _, tick := range ticks {
		playerData := tickData[tick]

		// Start new sequence on significant events
		startNewSequence := false
		if currentSeq == nil {
			startNewSequence = true
		} else {
			// Check for sequence-breaking events like:
			// - Round start/end
			// - Major engagements
			// - Significant utility usage
			// - Economic decisions
			if tick-currentSeq.startTick >= minSequenceLength {
				startNewSequence = t.isSignificantEvent(tick, playerData)
			}
		}

		if startNewSequence {
			if currentSeq != nil && currentSeq.endTick-currentSeq.startTick >= minSequenceLength {
				sequences = append(sequences, *currentSeq)
			}
			currentSeq = &sequence{
				startTick: tick,
				endTick:   tick,
				players:   make(map[uint64]bool),
			}
		}

		if currentSeq != nil {
			currentSeq.endTick = tick
			for playerID := range playerData {
				currentSeq.players[playerID] = true
			}
		}
	}

	// Add final sequence if valid
	if currentSeq != nil && currentSeq.endTick-currentSeq.startTick >= minSequenceLength {
		sequences = append(sequences, *currentSeq)
	}

	return sequences
}

// isSignificantEvent determines if a tick represents a significant game event
func (t *Trainer) isSignificantEvent(tick int, playerData map[uint64]types.PlayerTickData) bool {
	// Check for events like:
	// - Combat initiation
	// - Player deaths
	// - Utility usage
	// - Economic decisions
	for _, player := range playerData {
		if player.FiredActiveWeapon || len(player.DamageDealtToPlayer) > 0 {
			return true
		}
		// Add more event checks as needed
	}
	return false
}

// buildMapState analyzes the map state during a sequence
func (t *Trainer) buildMapState(seq sequence) MapState {
	state := MapState{}

	// Use map model to extract:
	// - Key areas and callouts
	// - Common angles and positions
	// - Cover points
	// - Tactical positions

	return state
}

// extractPlayerPositions extracts relevant player positions and movements
func (t *Trainer) extractPlayerPositions(seq sequence) []PlayerPosition {
	var positions []PlayerPosition

	// Extract key position data:
	// - Player movements
	// - Crosshair placement
	// - Viewangle changes
	// - Movement patterns

	return positions
}

// analyzeEconomy analyzes the economic state during a sequence
func (t *Trainer) analyzeEconomy(seq sequence, match *types.Match) EconomyState {
	state := EconomyState{
		Round: -1, // Will be set based on analysis
	}

	// Analyze:
	// - Team economy
	// - Buy patterns
	// - Save rounds
	// - Force buys
	// - Equipment value

	return state
}

// extractUtilityUsage extracts utility usage events
func (t *Trainer) extractUtilityUsage(seq sequence) []UtilityEvent {
	var events []UtilityEvent

	// Extract:
	// - Grenade usage
	// - Flash effectiveness
	// - Smoke placements
	// - Molotov usage

	return events
}

// extractCombatEvents extracts combat-related events
func (t *Trainer) extractCombatEvents(seq sequence) []CombatEvent {
	var events []CombatEvent

	// Extract:
	// - Kills/deaths
	// - Damage dealt
	// - Time to damage
	// - Crosshair placement
	// - Spray patterns

	return events
}

// TuneParameters performs hyperparameter tuning
func (t *Trainer) TuneParameters(ctx context.Context, tuningData []TrainingExample) error {
	t.logger.Info("Starting hyperparameter tuning")

	// Parameters to tune
	parameterSpace := map[string][]interface{}{
		"learning_rate": {0.001, 0.01, 0.1},
		"temperature":   {0.5, 0.7, 0.9},
		"top_p":         {0.8, 0.9, 0.95},
	}

	t.logger.Debug("Checking parameterSpace", "parameterSpace", parameterSpace)

	bestParams := make(map[string]interface{})
	bestScore := -1.0

	// Grid search through parameter combinations
	// TODO: Implement more sophisticated tuning strategy (e.g., Bayesian optimization)

	// Update model with best parameters
	t.model.Parameters = bestParams

	t.logger.Info("Completed parameter tuning",
		"best_params", bestParams,
		"best_score", bestScore)

	return nil
}
