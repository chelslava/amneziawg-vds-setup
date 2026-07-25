package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/chelslava/amneziawg-vds-setup/v2/internal/ssh"
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

type noticeModel struct {
	title string
	body  string
}

var errTUIBack = errors.New("interactive form cancelled")

type dropdownModel struct {
	title     string
	choices   []string
	selected  int
	cancelled bool
}

func (m dropdownModel) Init() tea.Cmd { return nil }

func (m dropdownModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return m, nil
	}
	switch key.String() {
	case "up", "k":
		if m.selected > 0 {
			m.selected--
		}
	case "down", "j":
		if m.selected < len(m.choices)-1 {
			m.selected++
		}
	case "enter", "space":
		return m, tea.Quit
	case "esc", "q":
		m.cancelled = true
		return m, tea.Quit
	}
	return m, nil
}

func (m dropdownModel) View() tea.View {
	var b strings.Builder
	b.WriteString("╭────────────────────────────────────────────╮\n")
	b.WriteString("│ awg-vds · select a value                   │\n")
	b.WriteString("╰────────────────────────────────────────────╯\n\n")
	b.WriteString(m.title)
	b.WriteString("\n\n")
	for i, choice := range m.choices {
		writeChoice(&b, i == m.selected, choice)
	}
	b.WriteString("\n↑/↓ or j/k · Enter select · Esc/back return")
	v := tea.NewView(b.String())
	v.AltScreen = true
	v.WindowTitle = "awg-vds · select"
	return v
}

func dropdown(in io.Reader, out io.Writer, title string, choices []string, selected int) (string, error) {
	final, err := tea.NewProgram(dropdownModel{title: title, choices: choices, selected: selected}, tea.WithInput(in), tea.WithOutput(out)).Run()
	if err != nil {
		return "", err
	}
	model := final.(dropdownModel)
	if model.cancelled {
		return "", errTUIBack
	}
	return model.choices[model.selected], nil
}

func (m noticeModel) Init() tea.Cmd { return nil }

func (m noticeModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return m, nil
	}
	if key.String() == "enter" || key.String() == "space" || key.String() == "q" || key.String() == "esc" {
		return m, tea.Quit
	}
	return m, nil
}

func (m noticeModel) View() tea.View {
	var b strings.Builder
	b.WriteString("╭────────────────────────────────────────────╮\n")
	b.WriteString("│ awg-vds · action needs attention            │\n")
	b.WriteString("╰────────────────────────────────────────────╯\n\n")
	b.WriteString("! ")
	b.WriteString(m.title)
	b.WriteString("\n\n")
	b.WriteString(m.body)
	b.WriteString("\n\nPress Enter to return to the main menu · q/Esc to dismiss")
	v := tea.NewView(b.String())
	v.AltScreen = true
	v.WindowTitle = "awg-vds · attention"
	return v
}

func showNoticeTUI(in io.Reader, out io.Writer, title, body string) error {
	_, err := tea.NewProgram(noticeModel{title: title, body: body}, tea.WithInput(in), tea.WithOutput(out)).Run()
	return err
}

func errorNotice(lang uiLanguage, action string, err error) (string, string) {
	title := "Operation failed"
	if lang == langRU {
		title = "Операция не выполнена"
	}
	detail := ssh.Redact(err.Error())
	recommendation := []string{}
	if strings.Contains(strings.ToLower(detail), "migration") || strings.Contains(strings.ToLower(detail), "legacy") && strings.Contains(strings.ToLower(detail), "upstream") {
		if lang == langRU {
			recommendation = []string{"На сервере уже установлен Legacy.", "Upstream — отдельный сценарий и не заменяет Legacy автоматически.", "Используйте чистый VDS для Upstream или повторите установку с engine legacy.", "Текущая конфигурация не изменена."}
		} else {
			recommendation = []string{"A Legacy installation already exists on this server.", "Upstream is a separate scenario and never replaces Legacy automatically.", "Use a fresh VDS for Upstream or retry with engine legacy.", "The existing configuration was not changed."}
		}
	} else if strings.Contains(strings.ToLower(detail), "amneziawg kernel module") || strings.Contains(strings.ToLower(detail), "module is neither installed nor available") {
		if lang == langRU {
			recommendation = []string{"Upstream требует модуль ядра AmneziaWG; текущая ОС или ядро его не поддерживает либо репозитории недоступны.", "Запустите doctor и проверьте версию ядра, архитектуру и доступность пакета/модуля amneziawg.", "Если провайдер не позволяет загрузить модуль, выберите engine legacy для совместимости с WireSock.", "Legacy и Upstream — разные сценарии; автоматической миграции между ними нет."}
		} else {
			recommendation = []string{"Upstream requires the AmneziaWG kernel module; this OS/kernel does not support it or the repositories are unavailable.", "Run doctor and check the kernel version, architecture, and availability of the amneziawg package/module.", "If the provider cannot load the module, choose engine legacy for WireSock compatibility.", "Legacy and Upstream are separate scenarios; automatic migration between them is not supported."}
		}
	} else if strings.Contains(strings.ToLower(detail), "module-load-failed") || strings.Contains(strings.ToLower(detail), "modprobe: fatal") {
		if lang == langRU {
			recommendation = []string{"Репозиторий найден, но модуль AmneziaWG не загрузился для текущего ядра.", "Проверьте linux-headers, dkms и результат: dkms status; dmesg | tail -50.", "Повторная установка обновлённой версии добавит headers/DKMS и попробует собрать модуль заново.", "Если headers или загрузка модулей запрещены провайдером, используйте engine legacy или другой VDS."}
		} else {
			recommendation = []string{"The repository was found, but the AmneziaWG module did not load for the running kernel.", "Check linux-headers, dkms, and run: dkms status; dmesg | tail -50.", "Retrying with the updated installer will add headers/DKMS and rebuild the module.", "If headers or module loading are blocked by the provider, use engine legacy or another VDS."}
		}
	} else if strings.Contains(strings.ToLower(detail), "ssh command failed") {
		if lang == langRU {
			recommendation = []string{"Проверьте адрес VDS, пользователя и порт SSH.", "При password выберите интерактивный ввод OpenSSH; пароль не передаётся в аргументах.", "Для ключа проверьте путь и known_hosts.", "Запустите doctor после исправления доступа."}
		} else {
			recommendation = []string{"Check the VDS address, SSH user, and SSH port.", "For password auth, use the interactive OpenSSH prompt; the password is never an argument.", "For key auth, check the identity path and known_hosts.", "Run doctor after fixing access."}
		}
	} else if strings.Contains(strings.ToLower(detail), "health") || action == "install" || action == "update" {
		if lang == langRU {
			recommendation = []string{"Запустите doctor и проверьте Docker, порты и firewall.", "Убедитесь, что UDP VPN-порт и TCP-порт панели свободны.", "Повторный install безопасно выполнит reconcile после устранения причины."}
		} else {
			recommendation = []string{"Run doctor and check Docker, ports, and the firewall.", "Make sure the VPN UDP port and panel TCP port are available.", "A later install safely reconciles the deployment after the cause is fixed."}
		}
	} else if lang == langRU {
		recommendation = []string{"Проверьте текст ошибки выше.", "Запустите doctor для диагностики сервера.", "После устранения причины повторите операцию."}
	} else {
		recommendation = []string{"Review the error above.", "Run doctor to diagnose the server.", "Retry the operation after fixing the cause."}
	}
	return title, "Error:\n" + detail + "\n\n" + strings.Join(recommendation, "\n")
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

// consoleInput reads one byte at a time so text prompts never read ahead of
// Bubble Tea's raw-mode keyboard reader on Windows.
type consoleInput struct{ in io.Reader }

func (r *consoleInput) ReadString(delim byte) (string, error) {
	var line []byte
	var one [1]byte
	for {
		n, err := r.in.Read(one[:])
		if n > 0 {
			line = append(line, one[0])
			if one[0] == delim {
				return string(line), nil
			}
		}
		if err != nil {
			return string(line), err
		}
	}
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
	reader := &consoleInput{in: in}
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
		command, err := interactiveCommandTUI(reader, in, out, selection.action, selection.language)
		if err != nil {
			if errors.Is(err, errTUIBack) {
				continue
			}
			title, body := errorNotice(selection.language, selection.action, err)
			if noticeErr := showNoticeTUI(in, out, title, body); noticeErr != nil {
				return noticeErr
			}
			continue
		}
		if (command[0] == "install" || command[0] == "update" || command[0] == "rotate-password") && !confirm(reader, out, tr(selection.language, "proceed")) {
			fmt.Fprintln(out, tr(selection.language, "cancelled"))
			continue
		}
		if err := runCommandWithPrompt(command, out, errOut, func() bool {
			return confirm(reader, out, tr(selection.language, "third_party_repo_prompt"))
		}); err != nil {
			title, body := errorNotice(selection.language, selection.action, err)
			if noticeErr := showNoticeTUI(in, out, title, body); noticeErr != nil {
				return noticeErr
			}
		} else {
			fmt.Fprintln(out, tr(selection.language, "operation_completed"))
			fmt.Fprintln(out, tr(selection.language, "return_menu"))
		}
	}
}
