package cmd

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/rajeshkannanramakrishnan/lv/internal/ui"
	"github.com/spf13/cobra"

	tea "github.com/charmbracelet/bubbletea"
)

const largeFileThreshold = 10 * 1024 * 1024 // 10MB

// Version is set at build time via -ldflags. Defaults to dev for local builds.
var Version = "dev"

func readLines(r io.Reader) ([]string, error) {
	br := bufio.NewReader(r)
	lines := make([]string, 0, 1024)

	for {
		line, err := br.ReadString('\n')
		if err != nil && err != io.EOF {
			return nil, err
		}

		if len(line) > 0 {
			line = strings.TrimSuffix(line, "\n")
			line = strings.TrimSuffix(line, "\r")
			lines = append(lines, line)
		}

		if err == io.EOF {
			break
		}
	}

	return lines, nil
}

var rootCmd = &cobra.Command{
	Use:   "lv [file]",
	Version: Version,
	Short: "High-performance TUI for log analysis",
	Long: `lv is a blazing fast terminal-based log viewer designed for developers and DevOps.

Key Features:
  - Instant loading of large files (GB+) via virtualization.
  - Interactive filtering (fuzzy & regex) and time-range drill-down.
  - "Time Travel": Jump directly to a specific timestamp (press 'J').
  - Stack trace folding for cleaner error analysis.
  - Follow mode (tail -f) with auto-scroll.
  - Mouse support for scrolling and selection.
  - Rich keyboard shortcuts (vim-like navigation).`,
	Example: `  # Open a local file
  lv app.log

  # Pipe logs from stdin
  kubectl logs -f my-pod | lv
  docker logs my-container | lv
  cat large.log | lv`,
	Args: cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		var lines []string
		var reader io.Reader

		if len(args) > 0 {
			// Read from file
			f, err := os.Open(args[0])
			if err != nil {
				fmt.Printf("Error opening file: %v\n", err)
				os.Exit(1)
			}
			defer f.Close()

			info, err := f.Stat()
			if err != nil {
				fmt.Printf("Error reading file info: %v\n", err)
				os.Exit(1)
			}

			if info.Size() > largeFileThreshold {
				// Stream large files to avoid startup stalls and memory spikes.
				reader = f
			} else {
				lines, err = readLines(f)
				if err != nil {
					fmt.Printf("Error reading file: %v\n", err)
					os.Exit(1)
				}
			}
		} else {
			// Check if stdin has data
			stat, _ := os.Stdin.Stat()
			if (stat.Mode() & os.ModeCharDevice) == 0 {
				// Stdin is a pipe. Pass it to the model for streaming.
				reader = os.Stdin
			} else {
				// No file and no stdin
				cmd.Help()
				os.Exit(0)
			}
		}

		filename := "Stdin"
		if len(args) > 0 {
			filename = args[0]
		}

		p := tea.NewProgram(ui.InitialModel(filename, lines, reader), tea.WithAltScreen(), tea.WithMouseCellMotion())
		if _, err := p.Run(); err != nil {
			fmt.Printf("Error running program: %v\n", err)
			os.Exit(1)
		}
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
