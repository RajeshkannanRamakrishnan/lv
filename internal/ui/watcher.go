package ui

import (
	"io"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/fsnotify/fsnotify"
)

type FileChangeMsg struct {
	NewContent string
	NewOffset  int64
	Error      error
}

// Global watcher to prevent creating multiple watchers if re-called (though ideally managed by model)
// For simplicity in Bubble Tea, we'll spawn a goroutine that waits.
// BUT, Bubble Tea commands are one-off functions.
// We need a way to blocking-wait until an event happens.

// WaitForFileChange waits for a write event on the file, then reads from the offset.
// It creates a new transient watcher for each wait to avoid complex state management
// in the functional CMD approach (or we could pass a long-lived watcher channel).
// Given "tail -f" typically just blocks, we can try a blocking approach.
//
// However, creating a new watcher every time is expensive.
// Better: Model holds the watcher, and we pass a channel to the Cmd?
// Or we just poll? `tail -f` often uses inotify.
//
// Let's try the channel approach.
// The Model will initialize the watcher.
// The Cmd will simply wait on `watcher.Events`.

func WaitForFileChange(watcher *fsnotify.Watcher, filename string, currentOffset int64) tea.Cmd {
	return func() tea.Msg {
		// Wait for an event
		// We need to select between watcher events and maybe a timeout/context?
		// Bubble Tea Cmds run in a goroutine.

		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return nil
				}
				if event.Name == filename && (event.Op&fsnotify.Write == fsnotify.Write) {
					// File written, read new content
					return readNewContent(filename, currentOffset)
				}
                // Handle rename/remove (log rotation)?
                if event.Name == filename && (event.Op&fsnotify.Rename == fsnotify.Rename || event.Op&fsnotify.Remove == fsnotify.Remove) {
                    // Start over or wait for recreate?
                    // For now, let's just wait and retry opening if it reappears
                    time.Sleep(1 * time.Second)
                    // If file exists again, likely rotated. Reset offset.
                    // This is complex. Let's stick to simple append for now.
                }

			case err, ok := <-watcher.Errors:
				if !ok {
					return nil
				}
				return FileChangeMsg{Error: err}
			}
		}
	}
}

func readNewContent(filename string, offset int64) tea.Msg {
	f, err := os.Open(filename)
	if err != nil {
		return FileChangeMsg{Error: err}
	}
	defer f.Close()

	_, err = f.Seek(offset, 0)
	if err != nil {
		return FileChangeMsg{Error: err}
	}

	content, err := io.ReadAll(f)
	if err != nil {
		return FileChangeMsg{Error: err}
	}

    newOffset := offset + int64(len(content))

	return FileChangeMsg{
		NewContent: string(content),
		NewOffset:  newOffset,
	}
}
