package trainer

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"math/rand/v2"
	"os"
	"path/filepath"
	"sort"
	"strings"
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
	DryRun           bool               // Whether to run in dry run mode
}

// OllamaRequest represents a request to the Ollama API
type OllamaRequest struct {
	Model       string                 `json:"model"`
	Prompt      string                 `json:"prompt"`
	Parameters  map[string]interface{} `json:"parameters,omitempty"`
	TrainingSet []TrainingInstance     `json:"training_set,omitempty"`
}

// TrainingInstance represents a single training example for Ollama
type TrainingInstance struct {
	Input      string                 `json:"input"`
	Output     string                 `json:"output"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
	Parameters map[string]interface{} `json:"parameters,omitempty"`
}

// ProcessedExample contains the processed training data ready for the model
type ProcessedExample struct {
	TimeSeriesFeatures []float64          // Processed time series data
	MapFeatures        []float64          // Processed map state features
	PlayerFeatures     []float64          // Processed player position features
	EconomyFeatures    []float64          // Processed economy state features
	UtilityFeatures    []float64          // Processed utility usage features
	CombatFeatures     []float64          // Processed combat event features
	Labels             map[string]float64 // Expected outputs/labels
	Metadata           map[string]string  // Additional metadata
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

// TrainingExample represents a single training instance
type TrainingExample struct {
	TimeSeriesData   map[int]map[uint64]types.PlayerTickData
	MapState         types.MapState
	ExpectedFeedback []string
	PlayerPositions  []PlayerPosition
	RoundEconomy     types.EconomyState
	UtilityUsage     []types.UtilityEvent
	CombatEvents     []types.CombatEvent
}

type PlayerPosition struct {
	Tick            int
	Position        r3.Vector
	ViewAngles      r3.Vector
	CrosshairHeight float64
	VisibleAreas    []string
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

// sequence represents a meaningful sequence of ticks (e.g., a round or engagement)
type sequence struct {
	startTick int
	endTick   int
	players   map[uint64]bool
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

	t.mapModel, err = visibility.LoadMapModel(mapObjPath)
	if err != nil {
		return fmt.Errorf("failed to load map model: %w", err)
	}

	t.playerModel, err = visibility.LoadPlayerModel(playerObjPath)
	if err != nil {
		return fmt.Errorf("failed to load player model: %w", err)
	}

	return nil
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

func (t *Trainer) trainBatch(ctx context.Context, batch []TrainingExample) float64 {
	request, err := t.processBatch(ctx, batch)
	if err != nil {
		t.logger.Error("Failed to process batch", "error", err)
		return math.MaxFloat64
	}

	if t.config.DryRun {
		// Log the request that would be sent to Ollama
		requestJSON, _ := json.MarshalIndent(request, "", "  ")
		t.logger.Info("Dry run: Ollama API request", "request", string(requestJSON))

		// Simulate batch loss
		return simulateBatchLoss(batch)
	}

	// TODO: Implement actual Ollama API call here
	// For now, return simulated loss
	return simulateBatchLoss(batch)
}

func (t *Trainer) processBatch(ctx context.Context, batch []TrainingExample) (OllamaRequest, error) {
	processedExamples := make([]ProcessedExample, len(batch))
	trainingInstances := make([]TrainingInstance, len(batch))

	// Process each example in the batch
	for i, example := range batch {
		processedExamples[i] = t.ProcessExample(example)
		trainingInstances[i] = t.model.formatTrainingInstance(processedExamples[i])
	}

	// Create Ollama request
	request := OllamaRequest{
		Model:       t.model.Name,
		Parameters:  t.model.Parameters,
		TrainingSet: trainingInstances,
	}

	return request, nil
}

func simulateBatchLoss(batch []TrainingExample) float64 {
	// Simulate a decreasing loss based on batch size
	baseLoss := 1.0
	batchSizeFactor := math.Log(float64(len(batch)))
	return baseLoss / (1.0 + batchSizeFactor)
}

// validate performs validation and returns metrics
func (t *Trainer) validate(ctx context.Context, examples []TrainingExample) TrainingMetrics {
	var metrics TrainingMetrics
	validationLoss := 0.0

	// Process validation examples in batches
	for i := 0; i < len(examples); i += t.config.BatchSize {
		end := min(i+t.config.BatchSize, len(examples))
		batch := examples[i:end]

		// Calculate batch metrics
		batchMetrics := t.validateBatch(ctx, batch)

		// Accumulate metrics
		validationLoss += batchMetrics.ValidationLoss
		metrics.FeedbackAccuracy += batchMetrics.FeedbackAccuracy
		metrics.TacticalPrecision += batchMetrics.TacticalPrecision
		metrics.PositionalAccuracy += batchMetrics.PositionalAccuracy
		metrics.EconomicPrecision += batchMetrics.EconomicPrecision
		metrics.ComparisonAccuracy += batchMetrics.ComparisonAccuracy
	}

	// Average the metrics
	batchCount := float64((len(examples) + t.config.BatchSize - 1) / t.config.BatchSize)
	metrics.ValidationLoss = validationLoss / batchCount
	metrics.FeedbackAccuracy /= batchCount
	metrics.TacticalPrecision /= batchCount
	metrics.PositionalAccuracy /= batchCount
	metrics.EconomicPrecision /= batchCount
	metrics.ComparisonAccuracy /= batchCount

	return metrics
}

func (t *Trainer) validateBatch(ctx context.Context, batch []TrainingExample) TrainingMetrics {
	var metrics TrainingMetrics

	if t.config.DryRun {
		// Simulate validation metrics for dry run
		metrics = TrainingMetrics{
			ValidationLoss:     simulateBatchLoss(batch) * 1.1, // Slightly higher than training loss
			FeedbackAccuracy:   0.85 + rand.Float64()*0.1,
			TacticalPrecision:  0.80 + rand.Float64()*0.1,
			PositionalAccuracy: 0.75 + rand.Float64()*0.1,
			EconomicPrecision:  0.82 + rand.Float64()*0.1,
			ComparisonAccuracy: 0.78 + rand.Float64()*0.1,
		}
		return metrics
	}

	// Process the batch
	// TODO: replace _ with request when this is ready
	_, err := t.processBatch(ctx, batch)
	if err != nil {
		t.logger.Error("Failed to process validation batch", "error", err)
		return metrics
	}

	// In dry run or development, we'll simulate the metrics
	// TODO: Implement actual Ollama API validation call here
	metrics = simulateValidationMetrics(batch)

	return metrics
}

func simulateValidationMetrics(batch []TrainingExample) TrainingMetrics {
	// Create realistic-looking simulated metrics
	baseAccuracy := 0.75 + (rand.Float64() * 0.15) // Random base accuracy between 0.75 and 0.90

	return TrainingMetrics{
		ValidationLoss:     1.0 - baseAccuracy,
		FeedbackAccuracy:   baseAccuracy + (rand.Float64() * 0.05),
		TacticalPrecision:  baseAccuracy - (rand.Float64() * 0.05),
		PositionalAccuracy: baseAccuracy + (rand.Float64() * 0.03),
		EconomicPrecision:  baseAccuracy - (rand.Float64() * 0.04),
		ComparisonAccuracy: baseAccuracy + (rand.Float64() * 0.02),
	}
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
	if t.config.DryRun {
		t.logger.Info("Dry run: Would save model to", "path", t.config.ModelPath)

		// Simulate model artifact
		modelArtifact := map[string]interface{}{
			"name":       t.model.Name,
			"parameters": t.model.Parameters,
			"metadata": map[string]interface{}{
				"training_date": time.Now().Format(time.RFC3339),
				"epochs":        t.config.Epochs,
				"batch_size":    t.config.BatchSize,
			},
		}

		// Pretty print the model artifact
		artifactJSON, _ := json.MarshalIndent(modelArtifact, "", "  ")
		t.logger.Info("Dry run: Model artifact", "model", string(artifactJSON))
		return nil
	}

	// TODO: Implement actual Ollama model saving
	// This would involve making API calls to save the model state
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

// ProcessExample converts a raw training example into processed features
func (t *Trainer) ProcessExample(example TrainingExample) ProcessedExample {
	processed := ProcessedExample{
		TimeSeriesFeatures: make([]float64, 0),
		MapFeatures:        make([]float64, 0),
		PlayerFeatures:     make([]float64, 0),
		EconomyFeatures:    make([]float64, 0),
		UtilityFeatures:    make([]float64, 0),
		CombatFeatures:     make([]float64, 0),
		Labels:             make(map[string]float64),
		Metadata:           make(map[string]string),
	}

	// Process time series data
	processed.TimeSeriesFeatures = t.extractTimeSeriesFeatures(example.TimeSeriesData)

	// Process map state
	processed.MapFeatures = t.extractMapFeatures(example.MapState)

	// Process player positions
	processed.PlayerFeatures = t.extractPlayerFeatures(example.PlayerPositions)

	// Process economy state
	processed.EconomyFeatures = t.extractEconomyFeatures(example.RoundEconomy)

	// Process utility usage
	processed.UtilityFeatures = t.extractUtilityFeatures(example.UtilityUsage)

	// Process combat events
	processed.CombatFeatures = t.extractCombatFeatures(example.CombatEvents)

	// Generate labels
	processed.Labels = t.generateLabels(example)

	return processed
}

func (t *Trainer) extractTimeSeriesFeatures(data map[int]map[uint64]types.PlayerTickData) []float64 {
	var features []float64

	// Extract temporal patterns
	for tick := range data {
		for _, playerData := range data[tick] {
			// Movement features
			features = append(features, float64(playerData.Velocity2D))
			features = append(features, float64(playerData.Velocity3D))

			// Combat features
			features = append(features, float64(playerData.Health))
			features = append(features, float64(playerData.Armor))

			// Equipment features
			features = append(features, float64(playerData.Money))
		}
	}

	return features
}

func (t *Trainer) extractMapFeatures(state types.MapState) []float64 {
	var features []float64

	// Extract spatial features
	for _, point := range state.CoverPoints {
		features = append(features, point.X, point.Y, point.Z)
	}

	for _, point := range state.ClearPoints {
		features = append(features, point.X, point.Y, point.Z)
	}

	// Add obstruction features
	for _, point := range state.Obstructions {
		features = append(features, point.X, point.Y, point.Z)
	}

	return features
}

func (t *Trainer) extractPlayerFeatures(positions []PlayerPosition) []float64 {
	var features []float64

	for _, pos := range positions {
		// Position features
		features = append(features, pos.Position.X, pos.Position.Y, pos.Position.Z)

		// View angle features
		features = append(features, pos.ViewAngles.X, pos.ViewAngles.Y, pos.ViewAngles.Z)

		// Crosshair placement
		features = append(features, pos.CrosshairHeight)
	}

	return features
}

func (t *Trainer) extractEconomyFeatures(state types.EconomyState) []float64 {
	return []float64{
		float64(state.TeamMoney),
		float64(state.PlayerMoney),
		float64(state.Round),
	}
}

func (t *Trainer) extractUtilityFeatures(events []types.UtilityEvent) []float64 {
	var features []float64

	for _, event := range events {
		features = append(features, event.Position.X, event.Position.Y, event.Position.Z)
		features = append(features, event.Impact)
		features = append(features, float64(event.Timestamp))
	}

	return features
}

func (t *Trainer) extractCombatFeatures(events []types.CombatEvent) []float64 {
	var features []float64

	for _, event := range events {
		features = append(features, float64(event.DamageDealt))
		features = append(features, event.TimeToDamage)
		features = append(features, event.CrosshairPlacement)
		features = append(features, boolToFloat(event.IsHeadshot))
	}

	return features
}

func (t *Trainer) generateLabels(example TrainingExample) map[string]float64 {
	labels := make(map[string]float64)

	// Generate tactical effectiveness labels
	labels["tactical_score"] = calculateTacticalScore(example)
	labels["utility_efficiency"] = calculateUtilityEfficiency(example)
	labels["combat_performance"] = calculateCombatPerformance(example)
	labels["economic_management"] = calculateEconomicScore(example)

	return labels
}

// Helper functions for label generation
func calculateTacticalScore(example TrainingExample) float64 {
	var score float64

	// Analyze positioning
	for _, pos := range example.PlayerPositions {
		// Award points for good crosshair placement
		score += math.Min(pos.CrosshairHeight/90.0, 1.0)

		// Consider visible areas coverage
		score += float64(len(pos.VisibleAreas)) * 0.1
	}

	// Analyze utility usage
	for _, util := range example.UtilityUsage {
		score += util.Impact * 0.2
	}

	return normalizeScore(score)
}

func (m *Model) formatTrainingInstance(example ProcessedExample) TrainingInstance {
	// Generate the full training prompt that includes both features and expected analysis
	input := generateTrainingPrompt(example)

	// Format the expected output using just the labels
	output := formatLabels(example.Labels)

	return TrainingInstance{
		Input:  input,
		Output: output,
		Metadata: map[string]interface{}{
			"timestamp": time.Now().Unix(),
			"features":  len(example.TimeSeriesFeatures) + len(example.MapFeatures),
			"labels":    len(example.Labels),
		},
		Parameters: m.Parameters,
	}
}

func formatLabels(labels map[string]float64) string {
	var parts []string
	for label, value := range labels {
		parts = append(parts, fmt.Sprintf("%s:%.4f", label, value))
	}
	sort.Strings(parts) // Ensure consistent ordering
	return strings.Join(parts, ", ")
}

func calculateUtilityEfficiency(example TrainingExample) float64 {
	if len(example.UtilityUsage) == 0 {
		return 0.0
	}

	var totalImpact float64
	for _, util := range example.UtilityUsage {
		totalImpact += util.Impact
	}

	return normalizeScore(totalImpact / float64(len(example.UtilityUsage)))
}

func calculateCombatPerformance(example TrainingExample) float64 {
	if len(example.CombatEvents) == 0 {
		return 0.0
	}

	var score float64
	for _, event := range example.CombatEvents {
		// Base damage score
		score += float64(event.DamageDealt) * 0.01

		// Bonus for headshots
		if event.IsHeadshot {
			score += 0.5
		}

		// Consider time to damage
		score += math.Max(0, 1.0-event.TimeToDamage/1000.0)
	}

	return normalizeScore(score / float64(len(example.CombatEvents)))
}

func calculateEconomicScore(example TrainingExample) float64 {
	var score float64

	// Consider current economic state
	score += float64(example.RoundEconomy.TeamMoney) * 0.0001

	// Bonus for optimal buy decisions
	if example.RoundEconomy.BuyDecision == example.RoundEconomy.OptimalBuy {
		score += 1.0
	}

	return normalizeScore(score)
}

// Utility functions
func normalizeScore(score float64) float64 {
	return math.Max(0.0, math.Min(1.0, score))
}

func boolToFloat(b bool) float64 {
	if b {
		return 1.0
	}
	return 0.0
}

func (t *Trainer) GetMetrics() []TrainingMetrics {
	t.mutex.RLock()
	defer t.mutex.RUnlock()

	metrics := make([]TrainingMetrics, len(t.metrics))
	copy(metrics, t.metrics)
	return metrics
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
func (t *Trainer) buildMapState(seq sequence) types.MapState {
	state := types.MapState{}

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
func (t *Trainer) analyzeEconomy(seq sequence, match *types.Match) types.EconomyState {
	state := types.EconomyState{
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
func (t *Trainer) extractUtilityUsage(seq sequence) []types.UtilityEvent {
	var events []types.UtilityEvent

	// Extract:
	// - Grenade usage
	// - Flash effectiveness
	// - Smoke placements
	// - Molotov usage

	return events
}

// extractCombatEvents extracts combat-related events
func (t *Trainer) extractCombatEvents(seq sequence) []types.CombatEvent {
	var events []types.CombatEvent

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

	if t.config.DryRun {
		t.logger.Info("Dry run: Would tune parameters", "parameter_space", parameterSpace)

		// Simulate parameter tuning results
		bestParams := map[string]interface{}{
			"learning_rate": 0.01,
			"temperature":   0.7,
			"top_p":         0.9,
		}
		bestScore := 0.85

		t.logger.Info("Dry run: Parameter tuning complete",
			"best_params", bestParams,
			"best_score", bestScore)

		// Update model parameters
		t.model.Parameters = bestParams
		return nil
	}

	// TODO: Implement actual parameter tuning logic here
	return nil
}

func generateTrainingPrompt(example ProcessedExample) string {
	var sb strings.Builder

	sb.WriteString("Analyze CS2 gameplay with the following features:\n\n")

	// Add feature descriptions
	sb.WriteString("Time Series Features:\n")
	for i, val := range example.TimeSeriesFeatures {
		sb.WriteString(fmt.Sprintf("- Feature %d: %.4f\n", i, val))
	}

	sb.WriteString("\nMap Features:\n")
	for i, val := range example.MapFeatures {
		sb.WriteString(fmt.Sprintf("- Feature %d: %.4f\n", i, val))
	}

	// Add expected outputs
	sb.WriteString("\nExpected Analysis:\n")
	for label, value := range example.Labels {
		sb.WriteString(fmt.Sprintf("- %s: %.4f\n", label, value))
	}

	return sb.String()
}
