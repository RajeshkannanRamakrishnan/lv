package cmd

import (
	"fmt"
    "io"
	"bufio"
	"os"

	"github.com/rajeshkannanramakrishnan/lv/internal/ui"
	"github.com/spf13/cobra"

	tea "github.com/charmbracelet/bubbletea"
)

var rootCmd = &cobra.Command{
	Use:   "lv [file]",
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
	Args:  cobra.MaximumNArgs(1),
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
            
            // For now, keep loading files into memory as per existing logic
            // (user request: "shouldn't break other changes")
            // We could stream files too, but let's be safe and keep large file loading behavior
            // which currently uses the lines slice.
            // Wait, root.go logic was:
			scanner := bufio.NewScanner(f)
            // Increase buffer for large lines
            buf := make([]byte, 0, 64*1024)
            scanner.Buffer(buf, 1024*1024)
            
			for scanner.Scan() {
				lines = append(lines, scanner.Text())
			}
            if err := scanner.Err(); err != nil {
                 fmt.Printf("Error reading file: %v\n", err)
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
