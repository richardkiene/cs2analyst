package progress

import (
	"fmt"
	"io"
	"os"
	"sync/atomic"

	"github.com/schollz/progressbar/v3"
)

type Config struct {
	Description string
	Total       int64
	ShowBytes   bool
	IsCounter   bool
}

// ProgressIndicator interface that both TickCounter and progressbar.ProgressBar implement
type ProgressIndicator interface {
	Add(n int) error
	Add64(n int64) error
	Finish() error
}

type TickCounter struct {
	count     int64
	writer    io.Writer
	lastCount int64
	spinChars []string
	spinIndex int
}

func NewTickCounter() *TickCounter {
	return &TickCounter{
		writer:    os.Stdout,
		spinChars: []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"},
	}
}

func (t *TickCounter) Add(n int) error {
	newCount := atomic.AddInt64(&t.count, int64(n))
	if newCount-t.lastCount > 1000 { // Update every 1000 ticks
		t.spinIndex = (t.spinIndex + 1) % len(t.spinChars)
		fmt.Printf("\r%s Processing ticks: %d", t.spinChars[t.spinIndex], newCount)
		t.lastCount = newCount
	}
	return nil
}

func (t *TickCounter) Add64(n int64) error {
	return t.Add(int(n))
}

func (t *TickCounter) Finish() error {
	fmt.Printf("\rProcessing ticks: %d (done)\n", atomic.LoadInt64(&t.count))
	return nil
}

func CreateProgressBar(cfg Config) ProgressIndicator {
	if cfg.IsCounter {
		return NewTickCounter()
	}

	// Normal progress bar for bounded operations
	options := []progressbar.Option{
		progressbar.OptionSetDescription(cfg.Description),
		progressbar.OptionSetTheme(progressbar.Theme{
			Saucer:        "=",
			SaucerHead:    ">",
			SaucerPadding: " ",
			BarStart:      "[",
			BarEnd:        "]",
		}),
		progressbar.OptionEnableColorCodes(true),
		progressbar.OptionSetWidth(30),
		progressbar.OptionShowCount(),
		progressbar.OptionOnCompletion(func() { fmt.Println() }),
	}

	if cfg.ShowBytes {
		options = append(options, progressbar.OptionShowBytes(true))
	}

	return progressbar.NewOptions64(
		cfg.Total,
		options...,
	)
}
