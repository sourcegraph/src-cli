package style

import (
	"fmt"
	"path/filepath"
	"text/tabwriter"

	"os"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/sourcegraph/sourcegraph/lib/errors"
	"golang.design/x/clipboard"
)

var baseStyle = lipgloss.NewStyle().
	BorderStyle(lipgloss.NormalBorder()).
	BorderForeground(lipgloss.Color("240"))

type model struct {
	table table.Model
}

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "q", "ctrl+c":
			return m, tea.Quit
		case "c":
			m.copyRowToClipboard(m.table.SelectedRow())
			copiedMessage := lipgloss.NewStyle().
				Foreground(lipgloss.Color("#32CD32")).
				Render(fmt.Sprintf(
					"Copied resource allocations for %s to clipboard",
					m.table.SelectedRow()[0],
				))
			return m, tea.Batch(
				tea.Printf(
					copiedMessage,
				),
			)
		case "C":
			tmpDir := os.TempDir()
			filePath := filepath.Join(tmpDir, "resource-dump.txt")
			m.dumpResources(m.table.Rows(), filePath)
			savedMessage := lipgloss.NewStyle().
				Foreground(lipgloss.Color("#32CD32")).
				Render(fmt.Sprintf(
					"saved resource allocations to %s",
					filePath,
				))
			return m, tea.Batch(
				tea.Printf(
                    savedMessage,
				),
			)
		}
	}
	m.table, cmd = m.table.Update(msg)
	return m, cmd
}

func (m model) View() string {
	s := "\n > Press 'j' and 'k' to go up and down\n"
	s += " > Press 'c' to copy highlighted row to clipboard\n"
    s += " > Press 'C' to copy all rows to a file\n"
	s += " > Press 'q' to quit\n\n"
	s += baseStyle.Render(m.table.View()) + "\n"
	return s
}

func (m model) dumpResources(rows []table.Row, filePath string) error {
	dumpFile, err := os.Create(filePath)
	if err != nil {
		return errors.Wrap(err, "error while creating new file")
	}
    
    defer func() {
        dumpFile.Close()
    }()

	tw := tabwriter.NewWriter(dumpFile, 0, 0, 3, ' ', 0)
	defer func() {
		tw.Flush()
	}()

    if len(rows[0]) == 6 {
        _, err = fmt.Fprintf(tw, fmt.Sprintf("NAME\tCPU LIMITS\tCPU REQUESTS\tMEM LIMITS\tMEM REQUESTS\tCAPACITY\n"))
        if err != nil {
            return errors.Wrap(err, "error while appending columns to filepath")
        }

        for _, row := range rows {
            name := row[0]
            cpuLimits := row[1]
            cpuRequests := row[2]
            memLimits := row[3]
            memRequests := row[4]
            capacity := row[5]
            fmt.Fprintf(
                tw,
                "%s\t%s\t%s\t%s\t%s\t%s\n",
                name,
                cpuLimits,
                cpuRequests,
                memLimits,
                memRequests,
                capacity,
            )
        }
    } else if len(rows[0]) == 5 {
        _, err = fmt.Fprintf(tw, fmt.Sprintf("NAME\tCPU CORES\tCPU SHARES\tMEM LIMITS\tMEM RESERVATIONS\n"))
        if err != nil {
            return errors.Wrap(err, "error while appending columns to filepath")
        }

        for _, row := range rows {
            name := row[0]
            cpuCores := row[1]
            cpuShares := row[2]
            memLimits := row[3]
            memReservations := row[4]
            fmt.Fprintf(
                tw,
                "%s\t%s\t%s\t%s\t%s\n",
                name,
                cpuCores,
                cpuShares,
                memLimits,
                memReservations,
            )
        }
    }

	return nil
}

func (m model) copyRowToClipboard(row table.Row) {
	var containerInfo string

	// change output based on the length of row
	// docker rows will always be length of 5
	// kubernetes rows will always be length of 6
	if len(row) == 5 {
		name := row[0]
		cpuCores := row[1]
		cpuShares := row[2]
		memLimits := row[3]
		memReservations := row[4]
		containerInfo = fmt.Sprintf(`container: %s
            cpu cores: %s 
            cpu shares: %s
            mem limits: %s
            mem reservations: %s`,
			name,
			cpuCores,
			cpuShares,
			memLimits,
			memReservations,
		)
	} else if len(row) == 6 {
		name := row[0]
		cpuLimits := row[1]
		cpuRequests := row[2]
		memLimits := row[3]
		memRequests := row[4]
		capacity := row[5]
		containerInfo = fmt.Sprintf(`container: %s
            cpu limits: %s 
            cpu requests: %s
            mem limits: %s
            mem requests: %s
            disk capacity: %s`,
			name,
			cpuLimits,
			cpuRequests,
			memLimits,
			memRequests,
			capacity,
		)
	}

	clipboard.Write(clipboard.FmtText, []byte(containerInfo))
}

func ResourceTable(columns []table.Column, rows []table.Row) {
	t := table.New(
		table.WithColumns(columns),
		table.WithRows(rows),
		table.WithFocused(true),
		table.WithHeight(14),
	)

	s := table.DefaultStyles()
	s.Header = s.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("240")).
		BorderBottom(true).
		Bold(false)
	s.Selected = s.Selected.
		Foreground(lipgloss.Color("229")).
		Background(lipgloss.Color("57")).
		Bold(false)
	t.SetStyles(s)

	m := model{t}
	if _, err := tea.NewProgram(m).Run(); err != nil {
		fmt.Println("Error running program:", err)
		os.Exit(1)
	}
}
