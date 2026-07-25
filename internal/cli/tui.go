package cli

import (
	"bufio"
	"fmt"
	"io"
	"strings"

	tea "charm.land/bubbletea/v2"
)

type uiLanguage string

const (
	langEN uiLanguage = "en"
	langRU uiLanguage = "ru"
)

type tuiSelection struct {
	language uiLanguage
	action   string
}

type tuiModel struct {
	step     int
	selected int
	choice   tuiSelection
}

var languageChoices = []string{"English", "Русский"}
var actionChoices = []string{"Install / reconcile", "Status", "Doctor", "Update", "Backup", "Rotate panel password", "CLI help", "Exit"}
var actionChoicesRU = []string{"Установка / reconcile", "Статус", "Диагностика", "Обновление", "Резервная копия", "Смена пароля панели", "Справка CLI", "Выход"}

func (m tuiModel) Init() tea.Cmd { return nil }

func (m tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return m, nil
	}
	switch key.String() {
	case "ctrl+c", "q", "esc":
		m.choice.action = "exit"
		return m, tea.Quit
	case "up", "k":
		if m.selected > 0 {
			m.selected--
		}
	case "down", "j":
		max := len(languageChoices)
		if m.step == 1 {
			max = len(actionChoices)
		}
		if m.selected < max-1 {
			m.selected++
		}
	case "enter", "space":
		if m.step == 0 {
			m.choice.language = langEN
			if m.selected == 1 {
				m.choice.language = langRU
			}
			m.step = 1
			m.selected = 0
		} else {
			m.choice.action = actionFromIndex(m.selected)
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m tuiModel) View() tea.View {
	var b strings.Builder
	b.WriteString("╭────────────────────────────────────────────╮\n")
	b.WriteString("│ awg-vds · AmneziaWG VDS control center     │\n")
	b.WriteString("╰────────────────────────────────────────────╯\n\n")
	if m.step == 0 {
		b.WriteString("Choose language / Выберите язык\n\n")
		for i, label := range languageChoices {
			writeChoice(&b, i == m.selected, label)
		}
		b.WriteString("\n↑/↓ or j/k · Enter · q/Esc")
	} else {
		if m.choice.language == langRU {
			b.WriteString("Главное меню\n\n")
			for i, label := range actionChoicesRU {
				writeChoice(&b, i == m.selected, label)
			}
			b.WriteString("\n↑/↓ или j/k · Enter · q/Esc")
		} else {
			b.WriteString("Main menu\n\n")
			for i, label := range actionChoices {
				writeChoice(&b, i == m.selected, label)
			}
			b.WriteString("\n↑/↓ or j/k · Enter · q/Esc")
		}
	}
	v := tea.NewView(b.String())
	v.AltScreen = true
	v.WindowTitle = "awg-vds"
	return v
}

func writeChoice(b *strings.Builder, selected bool, label string) {
	marker := "  "
	if selected {
		marker = "› "
	}
	fmt.Fprintf(b, "%s%s\n", marker, label)
}

func actionFromIndex(index int) string {
	return []string{"install", "status", "doctor", "update", "backup", "rotate-password", "help", "exit"}[index]
}

func runTUI(in io.Reader, out io.Writer) (tuiSelection, error) {
	final, err := tea.NewProgram(tuiModel{}, tea.WithInput(in), tea.WithOutput(out)).Run()
	if err != nil {
		return tuiSelection{}, err
	}
	return final.(tuiModel).choice, nil
}

func interactiveTUI(in io.Reader, out, errOut io.Writer) error {
	for {
		selection, err := runTUI(in, out)
		if err != nil {
			return err
		}
		if selection.action == "exit" || selection.action == "" {
			return nil
		}
		if selection.action == "help" {
			usage(out)
			continue
		}
		command, err := interactiveCommandLanguage(bufio.NewReader(in), out, selection.action, selection.language)
		if err != nil {
			fmt.Fprintln(out, tr(selection.language, "operation_failed"), err)
			continue
		}
		if (command[0] == "install" || command[0] == "update" || command[0] == "rotate-password") && !confirm(bufio.NewReader(in), out, tr(selection.language, "proceed")) {
			fmt.Fprintln(out, tr(selection.language, "cancelled"))
			continue
		}
		if err := runCommand(command, out, errOut); err != nil {
			fmt.Fprintln(out, tr(selection.language, "operation_failed"), err)
		} else {
			fmt.Fprintln(out, tr(selection.language, "operation_completed"))
		}
	}
}
