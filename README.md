# CS2 Analyst

Collects and analyzes Counter-Strike 2 demo files.

## Prerequisites

- Go 1.21 or higher
- CS2 demo files (.dem)
- Map .obj files
- Player model .obj files

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

```bash
cs2analyst --demo path/to/demo.dem \
          --maps path/to/maps/dir \
          --models path/to/models/dir \
          --output json \
          --output-path stats.json
```

### Required Flags

- `--demo`: Path to CS2 demo file
- `--maps`: Directory containing map .obj files
- `--models`: Directory containing player model .obj files
- `--output`: Output format (json, protobuf, or db)

### Optional Flags

- `--player`: Analyze specific player (default: all players)
- `--log-level`: Log level (debug, info, warn, error)
- `--output-path`: Path for json/protobuf output
- `--db-config`: Database configuration for db output

## Library Usage

```go
import (
    "github.com/richardkiene/cs2analyst/collector"
    "github.com/richardkiene/cs2analyst/analyzer"
    "github.com/richardkiene/cs2analyst/visibility"
)

// Create collector
c := collector.New(mapsDir, modelsDir)

// Process demo
data, err := c.ProcessDemo(demoPath, playerName)

// Analyze data
a := analyzer.New()
results, err := a.Analyze(data)
```

## License

MIT License - see LICENSE file