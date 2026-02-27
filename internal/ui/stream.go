package ui

import (
	"bufio"
	"io"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// LogChunkMsg can carry a chunk of raw string or a slice of lines.
// For efficient TUI updates, sending a batch of lines is preferred.
type LogChunkMsg struct {
	Lines []string
	Err   error
}

// Streamer manages the reading goroutine
type Streamer struct {
	lines chan []string
	err   chan error
}

func NewStreamer(r io.Reader) *Streamer {
	s := &Streamer{
		lines: make(chan []string),
		err:   make(chan error),
	}

	go func() {
		scanner := bufio.NewScanner(r)
		buf := make([]byte, 0, 64*1024)
		scanner.Buffer(buf, 16*1024*1024) // allow very large log lines
		var batch []string
		lastSend := time.Now()

		for scanner.Scan() {
			batch = append(batch, scanner.Text())

			// Flush if batch is big enough or time passed
			if len(batch) >= 100 || time.Since(lastSend) > 50*time.Millisecond {
				s.lines <- batch
				batch = nil // Reset
				lastSend = time.Now()
			}
		}

		// Flush remaining
		if len(batch) > 0 {
			s.lines <- batch
		}

		if err := scanner.Err(); err != nil {
			s.err <- err
		}

		close(s.lines)
		close(s.err)
	}()

	return s
}

// WaitForStream waits for the next batch from the channel
func WaitForStream(s *Streamer) tea.Cmd {
	return func() tea.Msg {
		select {
		case lines, ok := <-s.lines:
			if !ok {
				return nil // EOF
			}
			return LogChunkMsg{Lines: lines}
		case err, ok := <-s.err:
			if !ok {
				return nil
			}
			return LogChunkMsg{Err: err}
		}
	}
}
