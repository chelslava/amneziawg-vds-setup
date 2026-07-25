package cli

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
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
	step         int
	languageOnly bool
	selected     int
	choice       tuiSelection
}

var languageChoices = []string{"English", "Русский"}
var actionChoices = []string{"Install / reconcile", "Status", "Doctor", "Update", "Backup", "Rotate panel password", "Change language", "CLI help", "Exit"}
var actionChoicesRU = []string{"Установка / reconcile", "Статус", "Диагностика", "Обновление", "Резервная копия", "Смена пароля панели", "Сменить язык", "Справка CLI", "Выход"}

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
			if m.languageOnly {
				return m, tea.Quit
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
	return []string{"install", "status", "doctor", "update", "backup", "rotate-password", "change-language", "help", "exit"}[index]
}

func runTUI(in io.Reader, out io.Writer) (tuiSelection, error) {
	final, err := tea.NewProgram(tuiModel{step: 0, languageOnly: true}, tea.WithInput(in), tea.WithOutput(out)).Run()
	if err != nil {
		return tuiSelection{}, err
	}
	return final.(tuiModel).choice, nil
}

func runActionTUI(in io.Reader, out io.Writer, lang uiLanguage) (tuiSelection, error) {
	final, err := tea.NewProgram(tuiModel{step: 1, choice: tuiSelection{language: lang}}, tea.WithInput(in), tea.WithOutput(out)).Run()
	if err != nil {
		return tuiSelection{}, err
	}
	return final.(tuiModel).choice, nil
}

type uiPreferences struct {
	Language uiLanguage `json:"language"`
}

func languagePreferencePath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "awg-vds", "preferences.json"), nil
}

func loadLanguagePreference() (uiLanguage, error) {
	path, err := languagePreferencePath()
	if err != nil {
		return "", err
	}
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	var preferences uiPreferences
	if err := json.Unmarshal(b, &preferences); err != nil {
		return "", fmt.Errorf("read UI preferences: %w", err)
	}
	if preferences.Language != langEN && preferences.Language != langRU {
		return "", nil
	}
	return preferences.Language, nil
}

func saveLanguagePreference(lang uiLanguage) error {
	if lang != langEN && lang != langRU {
		return fmt.Errorf("unsupported UI language %q", lang)
	}
	path, err := languagePreferencePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(uiPreferences{Language: lang}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0600)
}

func interactiveTUI(in io.Reader, out, errOut io.Writer) error {
	language, err := loadLanguagePreference()
	if err != nil {
		language = langEN
	}
	if language == "" {
		selection, err := runTUI(in, out)
		if err != nil {
			return err
		}
		if selection.language == "" {
			return nil
		}
		language = selection.language
		if err := saveLanguagePreference(language); err != nil {
			fmt.Fprintln(out, "Warning: could not save UI language preference:", err)
		}
	}
	reader := bufio.NewReader(in)
	for {
		selection, err := runActionTUI(in, out, language)
		if err != nil {
			return err
		}
		if selection.action == "exit" || selection.action == "" {
			return nil
		}
		if selection.action == "change-language" {
			selected, err := runTUI(in, out)
			if err != nil {
				return err
			}
			if selected.language != "" {
				language = selected.language
				if err := saveLanguagePreference(language); err != nil {
					fmt.Fprintln(out, "Warning: could not save UI language preference:", err)
				}
			}
			continue
		}
		if selection.action == "help" {
			usage(out)
			continue
		}
		command, err := interactiveCommandLanguage(reader, out, selection.action, selection.language)
		if err != nil {
			fmt.Fprintln(out, tr(selection.language, "operation_failed"), err)
			continue
		}
		if (command[0] == "install" || command[0] == "update" || command[0] == "rotate-password") && !confirm(reader, out, tr(selection.language, "proceed")) {
			fmt.Fprintln(out, tr(selection.language, "cancelled"))
			continue
		}
		if err := runCommand(command, out, errOut); err != nil {
			fmt.Fprintln(out, tr(selection.language, "operation_failed"), err)
		} else {
			fmt.Fprintln(out, tr(selection.language, "operation_completed"))
			fmt.Fprintln(out, tr(selection.language, "return_menu"))
		}
	}
}
