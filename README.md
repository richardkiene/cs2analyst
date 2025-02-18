# CS2 Analyst

Collects and analyzes Counter-Strike 2 demo files. Supports both statistical analysis and AI-powered gameplay coaching.

## Prerequisites

- Go 1.21 or higher
- CS2 demo files (.dem)
- Map .obj files
- Player model .obj files
- Ollama (for training functionality)

## Installation

```bash
go install github.com/richardkiene/cs2analyst/cmd/cs2analyst@latest
```

## Building from Source

```bash
git clone https://github.com/richardkiene/cs2analyst.git
cd cs2analyst
go build ./cmd/cs2analyst
```

## Usage

CS2 Analyst provides two main commands: `analyze` for statistical analysis and `train` for AI model training.

### Analyze Command

Analyze CS2 demos and generate statistical insights:

```bash
cs2analyst analyze \
          --demo path/to/demo.dem \
          --maps path/to/maps/dir \
          --models path/to/models/dir \
          --output json \
          --output-path stats.json
```

#### Required Flags (analyze)

- `--demo`: Path to CS2 demo file
- `--maps`: Directory containing map .obj files
- `--models`: Directory containing player model .obj files
- `--output`: Output format (json, protobuf, or db)

#### Optional Flags (analyze)

- `--player`: Analyze specific player (default: all players)
- `--log-level`: Log level (debug, info, warn, error)
- `--output-path`: Path for json/protobuf output
- `--db-config`: Database configuration for db output
- `--debug-tick`: Generate debug visualization for specific tick
- `--debug-shooter`: SteamID of the shooter for debug visualization
- `--debug-target`: SteamID of the target for debug visualization

### Train Command

Train an AI model for gameplay analysis and coaching:

```bash
cs2analyst train \
          --demo path/to/demo.dem \
          --maps path/to/maps/dir \
          --models path/to/models/dir \
          --model-path output/model \
          --base-model codellama \
          --batch-size 32 \
          --epochs 100
```

#### Required Flags (train)

- `--demo`: Path to CS2 demo file for training
- `--maps`: Directory containing map .obj files
- `--models`: Directory containing player model .obj files
- `--model-path`: Path to save the trained model

#### Optional Flags (train)

- `--base-model`: Base model to use for training (default: codellama)
- `--batch-size`: Training batch size (default: 32)
- `--epochs`: Number of training epochs (default: 100)
- `--learning-rate`: Learning rate for training (default: 0.001)
- `--validation-split`: Portion of data to use for validation (default: 0.2)
- `--log-level`: Log level (debug, info, warn, error)

## Library Usage

```go
import (
    "github.com/richardkiene/cs2analyst/collector"
    "github.com/richardkiene/cs2analyst/analyzer"
    "github.com/richardkiene/cs2analyst/trainer"
    "github.com/richardkiene/cs2analyst/visibility"
)

// Create collector
c := collector.New()

// Process demo
match, err := c.Collect(demoPath)

// For statistical analysis
a := analyzer.New()
results, err := a.Analyze(c.PerTickInfo, match.TickRate, c.TickTime)

// For AI training
t, err := trainer.NewTrainer(trainer.TrainingConfig{
    BaseModel:  "codellama",
    ModelPath:  "output/model",
    BatchSize:  32,
    Epochs:     100,
})
examples, err := t.ConvertCollectorData(c.PerTickInfo, match)
err = t.Train(context.Background(), examples)
```

## AI Training Output

The trained model can provide insights such as:

- Tactical mistakes and missed opportunities
- Crosshair placement analysis
- Economic decision evaluation
- Movement and positioning feedback
- Utility usage optimization
- Comparative analysis with pro-level play