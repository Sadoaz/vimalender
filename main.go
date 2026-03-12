package main

import (
	"fmt"
	"os"

	"github.com/Sadoaz/vimalender/internal"
	tea "github.com/charmbracelet/bubbletea"
)

var subcommands = map[string]func([]string) error{
	"list":   internal.RunList,
	"add":    internal.RunAdd,
	"delete": internal.RunDelete,
	"search": internal.RunSearch,
	"import": internal.RunImport,
	"export": internal.RunExport,
}

func main() {
	if len(os.Args) > 1 {
		cmd := os.Args[1]
		if fn, ok := subcommands[cmd]; ok {
			if err := fn(os.Args[2:]); err != nil {
				fmt.Fprintf(os.Stderr, "error: %v\n", err)
				os.Exit(1)
			}
			return
		}
		// Unknown subcommand
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\nAvailable commands:\n", cmd)
		fmt.Fprintf(os.Stderr, "  list      List events for a date or range\n")
		fmt.Fprintf(os.Stderr, "  add       Create a new event\n")
		fmt.Fprintf(os.Stderr, "  delete    Delete an event by ID\n")
		fmt.Fprintf(os.Stderr, "  search    Search events by title or description\n")
		fmt.Fprintf(os.Stderr, "  import    Import events from an .ics file\n")
		fmt.Fprintf(os.Stderr, "  export    Export events to an .ics file\n")
		fmt.Fprintf(os.Stderr, "\nRun with no arguments to launch the TUI.\n")
		os.Exit(1)
	}

	p := tea.NewProgram(
		internal.NewModel(),
		tea.WithAltScreen(),
	)

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
