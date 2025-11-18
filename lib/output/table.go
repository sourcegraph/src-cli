package output

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
)

const (
	purple    = lipgloss.Color("99")
	gray      = lipgloss.Color("245")
	lightGray = lipgloss.Color("241")
)

type Table struct {
	tbl *table.Table

	headers []string
}

func NewTable(headers []string) *Table {
	t := table.New().
		Border(lipgloss.NormalBorder()).
		BorderStyle(lipgloss.NewStyle().Foreground(purple)).
		BorderRow(true).
		BorderColumn(true).
		StyleFunc(func(row, col int) lipgloss.Style {
			s := lipgloss.NewStyle().Width(30)

			// this is the stle to use for the header row
			if row == 0 {
				return s.Foreground(purple).Bold(true).Align(lipgloss.Center)
			}
			return s
		}).
		Headers(headers...)
	return &Table{
		tbl:     t,
		headers: headers,
	}
}

func (t *Table) AddRow(elems ...string) {
	t.tbl.Row(elems...)
}

func (t *Table) AddRows(elems ...[]string) {
	t.tbl.Rows(elems...)
}

func (t *Table) Render() string {
	return t.tbl.Render()
}
