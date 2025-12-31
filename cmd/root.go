package cmd

import (
	"fmt"
	"io/ioutil"
	"os"

	"github.com/rajeshkannanramakrishnan/lv/internal/ui"
	"github.com/spf13/cobra"

	tea "github.com/charmbracelet/bubbletea"
)

var rootCmd = &cobra.Command{
	Use:   "lv [file]",
	Short: "Log Viewer is a TUI for viewing log files",
	Long:  `A fast and interactive Log Viewer built with Bubbletea.`,
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		var content string

		if len(args) > 0 {
			// Read from file
			b, err := ioutil.ReadFile(args[0])
			if err != nil {
				fmt.Printf("Error reading file: %v\n", err)
				os.Exit(1)
			}
			content = string(b)
		} else {
			// Check if stdin has data
			stat, _ := os.Stdin.Stat()
			if (stat.Mode() & os.ModeCharDevice) == 0 {
				b, err := ioutil.ReadAll(os.Stdin)
				if err != nil {
					fmt.Printf("Error reading stdin: %v\n", err)
					os.Exit(1)
				}
				content = string(b)
				args = append(args, "Stdin") // Hack to reuse filename var if needed or just pass string
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

		p := tea.NewProgram(ui.InitialModel(filename, content), tea.WithAltScreen(), tea.WithMouseCellMotion())
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
