package skilldrop

import (
	"errors"
	"fmt"
	"io"

	tea "github.com/charmbracelet/bubbletea"
)

var errPickerCanceled = errors.New("selection canceled")

type pickerItem struct {
	Label  string
	Detail string
}

type pickerModel struct {
	title    string
	items    []pickerItem
	cursor   int
	selected int
	canceled bool
}

func newPickerModel(title string, items []pickerItem) pickerModel {
	return pickerModel{title: title, items: items, selected: -1}
}

func (m pickerModel) Init() tea.Cmd {
	return nil
}

func (m pickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch key.String() {
	case "esc", "ctrl+c", "q":
		m.canceled = true
		return m, tea.Quit
	case "up", "k":
		m.cursor = clampIndex(m.cursor-1, len(m.items))
	case "down", "j":
		m.cursor = clampIndex(m.cursor+1, len(m.items))
	case "enter":
		m.selected = m.cursor
		return m, tea.Quit
	}
	return m, nil
}

func (m pickerModel) View() string {
	out := m.title + "\n\n"
	for i, item := range m.items {
		cursor := " "
		if i == m.cursor {
			cursor = ">"
		}
		line := fmt.Sprintf("%s %s", cursor, item.Label)
		if item.Detail != "" {
			line += "  " + item.Detail
		}
		if i == m.cursor {
			line = tuiAccentStyle().Render(line)
		}
		out += line + "\n"
	}
	out += "\nenter select  up/down move  esc cancel\n"
	return out
}

func runInlinePicker(out io.Writer, title string, items []pickerItem) (int, error) {
	if len(items) == 0 {
		return -1, &ExitError{Code: ExitInvalidUsage, Err: errors.New("no options available")}
	}
	program := tea.NewProgram(newPickerModel(title, items), tea.WithOutput(out))
	finalModel, err := program.Run()
	if err != nil {
		return -1, &ExitError{Code: ExitGeneral, Err: err}
	}
	model, ok := finalModel.(pickerModel)
	if !ok {
		return -1, &ExitError{Code: ExitGeneral, Err: errors.New("unexpected picker model")}
	}
	if model.canceled || model.selected < 0 {
		return -1, &ExitError{Code: ExitInvalidUsage, Err: errPickerCanceled}
	}
	return model.selected, nil
}
