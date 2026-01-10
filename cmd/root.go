package cmd

import (
	"fmt"
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

		if len(args) > 0 {
			// Read from file
			f, err := os.Open(args[0])
			if err != nil {
				fmt.Printf("Error opening file: %v\n", err)
				os.Exit(1)
			}
			defer f.Close()

			reader := bufio.NewReader(f)
			for {
				line, err := reader.ReadString('\n')
				if len(line) > 0 {
					// Trim newline manually as ReadString includes it, unlike Scanner.Text()
					if line[len(line)-1] == '\n' {
						line = line[:len(line)-1]
						if len(line) > 0 && line[len(line)-1] == '\r' {
							line = line[:len(line)-1]
						}
					}
					lines = append(lines, line)
				}
				if err != nil {
					if err.Error() != "EOF" {
						fmt.Printf("Error reading file: %v\n", err)
						os.Exit(1)
					}
					break
				}
			}
		} else {
			// Check if stdin has data
			stat, _ := os.Stdin.Stat()
			if (stat.Mode() & os.ModeCharDevice) == 0 {
				reader := bufio.NewReader(os.Stdin)
				for {
					line, err := reader.ReadString('\n')
					if len(line) > 0 {
                        // Trim newline
						if line[len(line)-1] == '\n' {
							line = line[:len(line)-1]
							if len(line) > 0 && line[len(line)-1] == '\r' {
								line = line[:len(line)-1]
							}
						}
						lines = append(lines, line)
					}
					if err != nil {
						if err.Error() != "EOF" {
							fmt.Printf("Error reading stdin: %v\n", err)
							os.Exit(1)
						}
						break
					}
				}
				args = append(args, "Stdin") 
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

		p := tea.NewProgram(ui.InitialModel(filename, lines), tea.WithAltScreen(), tea.WithMouseCellMotion())
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
