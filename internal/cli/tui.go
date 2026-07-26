package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/chelslava/amneziawg-vds-setup/v2/internal/config"
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

func wrapText(value string, width int) []string {
	if width < 1 {
		return []string{value}
	}
	var result []string
	for _, paragraph := range strings.Split(value, "\n") {
		words := strings.Fields(paragraph)
		if len(words) == 0 {
			result = append(result, "")
			continue
		}
		line := words[0]
		for _, word := range words[1:] {
			if len([]rune(line))+1+len([]rune(word)) > width {
				result = append(result, line)
				line = word
			} else {
				line += " " + word
			}
		}
		result = append(result, line)
	}
	return result
}

type noticeModel struct {
	title    string
	body     string
	options  []noticeOption
	selected int
	choice   string
}

type noticeOption struct {
	action string
	label  string
}

type summaryModel struct {
	language  uiLanguage
	lines     []string
	confirmed bool
}

func (m summaryModel) Init() tea.Cmd { return nil }

func (m summaryModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return m, nil
	}
	switch key.String() {
	case "enter", "space", "y", "у":
		m.confirmed = true
		return m, tea.Quit
	case "esc", "q", "n", "т":
		return m, tea.Quit
	}
	return m, nil
}

func (m summaryModel) View() tea.View {
	var b strings.Builder
	b.WriteString("╭────────────────────────────────────────────╮\n")
	if m.language == langRU {
		b.WriteString("│ awg-vds · подтверждение операции            │\n")
	} else {
		b.WriteString("│ awg-vds · confirm operation                 │\n")
	}
	b.WriteString("╰────────────────────────────────────────────╯\n\n")
	if m.language == langRU {
		b.WriteString("Проверьте параметры операции\n\n")
	} else {
		b.WriteString("Review operation parameters\n\n")
	}
	for _, line := range m.lines {
		b.WriteString("  ")
		b.WriteString(line)
		b.WriteByte('\n')
	}
	b.WriteString("\n")
	if m.language == langRU {
		b.WriteString("Enter — продолжить · Esc — назад")
	} else {
		b.WriteString("Enter — continue · Esc — back")
	}
	v := tea.NewView(b.String())
	v.AltScreen = true
	v.WindowTitle = "awg-vds · confirm"
	return v
}

func showSummaryTUI(in io.Reader, out io.Writer, lang uiLanguage, args []string) (bool, error) {
	final, err := tea.NewProgram(summaryModel{language: lang, lines: operationSummary(args, lang)}, tea.WithInput(in), tea.WithOutput(out)).Run()
	if err != nil {
		return false, err
	}
	return final.(summaryModel).confirmed, nil
}

func operationSummary(args []string, lang uiLanguage) []string {
	values := map[string]string{}
	for i := 1; i+1 < len(args); i++ {
		if strings.HasPrefix(args[i], "--") {
			values[args[i]] = args[i+1]
			i++
		}
	}
	get := func(key, fallback string) string {
		if value := values[key]; value != "" {
			return value
		}
		return fallback
	}
	auth := "password"
	if values["--identity-file"] != "" {
		auth = "key"
	}
	tls := "disabled"
	for _, arg := range args {
		if arg == "--tls" {
			tls = "enabled"
		}
	}
	timeout := get("--timeout", "120")
	if _, explicit := values["--timeout"]; !explicit {
		if args[0] == "install" {
			timeout = "1800"
		}
	}
	risk := ""
	if args[0] == "install" && get("--engine", "legacy") == "upstream" {
		risk = "APT full-upgrade: enabled; AmneziaWG PPA: ask if required"
	}
	if get("--domain", "") == "" && args[0] == "install" {
		risk += " | WARNING: panel HTTP without domain/TLS"
	}
	if lang == langRU {
		return []string{"Команда: " + args[0], "VDS: " + get("--host", "-"), "SSH: " + get("--user", "root") + ":" + get("--ssh-port", "22") + " (" + auth + ")", "Движок: " + get("--engine", "legacy"), "VPN UDP: " + get("--vpn-port", "1234"), "Панель TCP: " + get("--web-port", "51821"), "Домен: " + get("--domain", "нет"), "TLS: " + tls, "Timeout: " + timeout + " sec", risk}
	}
	return []string{"Command: " + args[0], "VDS: " + get("--host", "-"), "SSH: " + get("--user", "root") + ":" + get("--ssh-port", "22") + " (" + auth + ")", "Engine: " + get("--engine", "legacy"), "VPN UDP: " + get("--vpn-port", "1234"), "Panel TCP: " + get("--web-port", "51821"), "Domain: " + get("--domain", "none"), "TLS: " + tls, "Timeout: " + timeout + " sec", risk}
}

type operationMessage struct {
	text   string
	done   bool
	err    error
	prompt chan bool
}

type operationTick time.Time

type operationModel struct {
	action      string
	language    uiLanguage
	lines       []string
	width       int
	startedAt   time.Time
	finishedAt  time.Time
	done        bool
	cancelling  bool
	logOffset   int
	savedPath   string
	err         error
	cancel      context.CancelFunc
	prompt      chan bool
	messageChan <-chan operationMessage
}

func (m operationModel) Init() tea.Cmd {
	return tea.Batch(waitOperationMessage(m.messageChan), tickOperation())
}

func (m operationModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch value := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = value.Width
		return m, nil
	case operationTick:
		if m.done {
			return m, nil
		}
		return m, tickOperation()
	case operationMessage:
		if value.prompt != nil {
			m.prompt = value.prompt
			return m, waitOperationMessage(m.messageChan)
		}
		if value.text != "" {
			for _, line := range strings.Split(strings.ReplaceAll(value.text, "\r\n", "\n"), "\n") {
				if strings.TrimSpace(line) != "" {
					m.lines = append(m.lines, timestampOperationLogLine(time.Now(), ssh.Redact(line)))
				}
			}
			if len(m.lines) > 500 {
				m.lines = m.lines[len(m.lines)-500:]
			}
			m.logOffset = 0
		}
		if value.done {
			m.done = true
			m.finishedAt = time.Now()
			m.err = value.err
		}
		return m, waitOperationMessage(m.messageChan)
	case tea.KeyPressMsg:
		if m.prompt != nil {
			switch value.String() {
			case "y", "у", "enter", "space":
				m.prompt <- true
				m.prompt = nil
			case "n", "т", "esc", "q":
				m.prompt <- false
				m.prompt = nil
			}
			return m, nil
		}
		if m.done {
			if value.String() == "s" {
				m.savedPath = saveOperationLog(m.lines)
				return m, nil
			}
			if value.String() == "up" || value.String() == "k" || value.String() == "pgup" {
				m.logOffset++
				return m, nil
			}
			if value.String() == "down" || value.String() == "j" || value.String() == "pgdown" {
				if m.logOffset > 0 {
					m.logOffset--
				}
				return m, nil
			}
			if value.String() == "enter" || value.String() == "space" || value.String() == "q" || value.String() == "esc" {
				return m, tea.Quit
			}
			return m, nil
		}
		if value.String() == "up" || value.String() == "k" || value.String() == "pgup" {
			m.logOffset++
			return m, nil
		}
		if value.String() == "down" || value.String() == "j" || value.String() == "pgdown" {
			if m.logOffset > 0 {
				m.logOffset--
			}
			return m, nil
		}
		if value.String() == "ctrl+c" || value.String() == "c" || value.String() == "esc" {
			m.cancelling = true
			if m.cancel != nil {
				m.cancel()
			}
		}
	}
	return m, nil
}

func (m operationModel) View() tea.View {
	width := m.width
	if width < 60 {
		width = 60
	}
	status := "running"
	if m.cancelling {
		status = "cancelling"
	} else if m.done && m.err != nil {
		status = "failed"
	} else if m.done {
		status = "completed"
	}
	end := time.Now()
	if !m.finishedAt.IsZero() {
		end = m.finishedAt
	}
	elapsed := end.Sub(m.startedAt).Round(time.Second)
	var b strings.Builder
	fmt.Fprintf(&b, "╭%s╮\n", strings.Repeat("─", width-2))
	fmt.Fprintf(&b, "│ awg-vds · %-*s│\n", width-17, "operation")
	fmt.Fprintf(&b, "╰%s╯\n\n", strings.Repeat("─", width-2))
	fmt.Fprintf(&b, "Action: %s\nStatus: %s\nElapsed: %s\n\n", m.action, status, elapsed)
	if m.prompt != nil {
		if m.language == langRU {
			b.WriteString("Добавить официальный PPA AmneziaWG?\nИзменятся APT-источники сервера. [y] да / [n] нет\n\n")
		} else {
			b.WriteString("Add the official AmneziaWG PPA?\nThis changes the server APT sources. [y] yes / [n] no\n\n")
		}
	}
	if m.savedPath != "" {
		fmt.Fprintf(&b, "Log saved: %s\n\n", m.savedPath)
	}
	b.WriteString("Logs:\n")
	logEnd := len(m.lines) - (m.logOffset * 14)
	if logEnd < 0 {
		logEnd = 0
	}
	start := logEnd - 14
	if start < 0 {
		start = 0
	}
	for _, line := range m.lines[start:logEnd] {
		for _, wrapped := range wrapText(formatOperationLog(line), width-4) {
			fmt.Fprintf(&b, "  %s\n", wrapped)
		}
	}
	b.WriteString("\n")
	if m.done {
		b.WriteString("↑/↓ logs · s save · Press Enter to continue · q/Esc to return")
	} else if m.language == langRU {
		b.WriteString("↑/↓ логи · c/Ctrl+C отменить · Esc отменить")
	} else {
		b.WriteString("↑/↓ logs · c/Ctrl+C cancel · Esc cancel")
	}
	v := tea.NewView(b.String())
	v.AltScreen = true
	v.WindowTitle = "awg-vds · operation"
	return v
}

func formatOperationLog(line string) string {
	if key, value, ok := strings.Cut(line, "="); ok && key != "" {
		return strings.ReplaceAll(key, "_", " ") + ": " + value
	}
	return line
}

func timestampOperationLogLine(now time.Time, line string) string {
	return "[" + now.Format("15:04:05") + "] " + line
}
func saveOperationLog(lines []string) string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	dir = filepath.Join(dir, "awg-vds", "logs")
	if os.MkdirAll(dir, 0700) != nil {
		return ""
	}
	path := filepath.Join(dir, "operation-"+time.Now().Format("20060102-150405")+".log")
	var b strings.Builder
	for _, line := range lines {
		b.WriteString(ssh.Redact(line))
		b.WriteByte('\n')
	}
	if os.WriteFile(path, []byte(b.String()), 0600) != nil {
		return ""
	}
	return path
}

func tickOperation() tea.Cmd {
	return tea.Tick(time.Second, func(now time.Time) tea.Msg { return operationTick(now) })
}

func waitOperationMessage(ch <-chan operationMessage) tea.Cmd {
	return func() tea.Msg { return <-ch }
}

type operationWriter struct{ ch chan<- operationMessage }

func (operationWriter) StreamedOutput() bool { return true }

func (w operationWriter) Write(p []byte) (int, error) {
	if len(p) > 0 {
		w.ch <- operationMessage{text: string(p)}
	}
	return len(p), nil
}

func runOperationTUI(in io.Reader, out io.Writer, args []string, lang uiLanguage, password string) error {
	ctx, cancel := context.WithCancel(context.Background())
	messages := make(chan operationMessage, 256)
	writer := operationWriter{ch: messages}
	prompt := func() bool {
		decision := make(chan bool, 1)
		select {
		case messages <- operationMessage{prompt: decision}:
		case <-ctx.Done():
			return false
		}
		select {
		case answer := <-decision:
			return answer
		case <-ctx.Done():
			return false
		}
	}
	go func() {
		err := runCommandWithPromptContext(ctx, args, writer, writer, prompt, password)
		messages <- operationMessage{done: true, err: err}
	}()
	final, err := tea.NewProgram(operationModel{action: args[0], language: lang, startedAt: time.Now(), cancel: cancel, messageChan: messages}, tea.WithInput(in), tea.WithOutput(out)).Run()
	cancel()
	if err != nil {
		return err
	}
	return final.(operationModel).err
}

func installAccessNotice(lang uiLanguage, args []string, password string) string {
	o, _, err := parse(args)
	if err != nil {
		return panelPasswordNotice(lang, password)
	}
	panelURL := installPanelURL(o)
	sshCommand := fmt.Sprintf("ssh %s@%s", o.User, o.Host)
	if o.SSHPort != 22 {
		sshCommand = fmt.Sprintf("ssh -p %d %s@%s", o.SSHPort, o.User, o.Host)
	}
	readPasswordCommand := sshCommand + " sudo cat /opt/awg-vds/panel-password"
	if lang == langRU {
		return fmt.Sprintf("Данные доступа к установленному стенду:\n\nПанель: %s\nЛогин панели: не требуется\nПароль панели: %s\nVPN UDP: %d\nEngine: %s\nSSH: %s\nПароль панели на сервере: /opt/awg-vds/panel-password\nПовторно прочитать: %s\n\nСохраните эти данные в менеджере паролей. awg-vds не пишет пароль панели в operation logs, state или флаговый CLI-вывод.", panelURL, password, o.VPNPort, o.Engine, sshCommand, readPasswordCommand)
	}
	return fmt.Sprintf("Installed access details:\n\nPanel: %s\nPanel login: not required\nPanel password: %s\nVPN UDP: %d\nEngine: %s\nSSH: %s\nPanel password file: /opt/awg-vds/panel-password\nRead again: %s\n\nSave these details in a password manager. awg-vds does not write the panel password to operation logs, state, or flag-mode CLI output.", panelURL, password, o.VPNPort, o.Engine, sshCommand, readPasswordCommand)
}

func installPanelURL(o config.Options) string {
	if o.TLS && o.Domain != "" {
		return "https://" + o.Domain
	}
	host := o.Domain
	if host == "" {
		host = o.Host
	}
	return fmt.Sprintf("http://%s:%d", host, o.WebPort)
}

func panelPasswordNotice(lang uiLanguage, password string) string {
	if lang == langRU {
		return fmt.Sprintf("Пароль панели (показывается только здесь):\n\n%s\n\nСохраните его в менеджере паролей; awg-vds больше нигде его не выводит.", password)
	}
	return fmt.Sprintf("Panel password (shown only here):\n\n%s\n\nSave it in a password manager; awg-vds never prints it elsewhere.", password)
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
	b.WriteString("\n↑/↓ or j/k · Enter/Ввод · Esc/back return · назад")
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
	if len(m.options) > 0 {
		switch key.String() {
		case "up", "k":
			if m.selected > 0 {
				m.selected--
			}
		case "down", "j":
			if m.selected < len(m.options)-1 {
				m.selected++
			}
		case "enter", "space":
			m.choice = m.options[m.selected].action
			return m, tea.Quit
		case "q", "esc":
			m.choice = "back"
			return m, tea.Quit
		}
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
	b.WriteString("\n\nEnter/Ввод — return to menu · q/Esc — dismiss")
	if len(m.options) > 0 {
		b.WriteString("\n\n")
		for i, option := range m.options {
			marker := "  "
			if i == m.selected {
				marker = "› "
			}
			fmt.Fprintf(&b, "%s%s\n", marker, option.label)
		}
		b.WriteString("\n↑/↓ · Enter select · Esc/back")
	}
	v := tea.NewView(b.String())
	v.AltScreen = true
	v.WindowTitle = "awg-vds · attention"
	return v
}

func showNoticeTUI(in io.Reader, out io.Writer, title, body string) error {
	_, err := tea.NewProgram(noticeModel{title: title, body: body}, tea.WithInput(in), tea.WithOutput(out)).Run()
	return err
}

func showRecoveryNoticeTUI(in io.Reader, out io.Writer, lang uiLanguage, action string, err error, title, body string) (string, error) {
	detail := strings.ToLower(err.Error())
	options := []noticeOption{{action: "retry", label: localizedAction(lang, "retry")}}
	options = append(options, noticeOption{action: "doctor", label: localizedAction(lang, "doctor")})
	if action == "install" && strings.Contains(detail, "upstream") && (strings.Contains(detail, "amneziawg") || strings.Contains(detail, "kernel") || strings.Contains(detail, "module")) {
		options = append(options, noticeOption{action: "legacy", label: localizedAction(lang, "legacy")})
	}
	options = append(options, noticeOption{action: "back", label: localizedAction(lang, "back")})
	final, runErr := tea.NewProgram(noticeModel{title: title, body: body, options: options}, tea.WithInput(in), tea.WithOutput(out)).Run()
	if runErr != nil {
		return "back", runErr
	}
	return final.(noticeModel).choice, nil
}

func localizedAction(lang uiLanguage, action string) string {
	if lang == langRU {
		switch action {
		case "retry":
			return "Повторить"
		case "doctor":
			return "Запустить doctor"
		case "legacy":
			return "Использовать legacy"
		default:
			return "Назад"
		}
	}
	switch action {
	case "retry":
		return "Retry"
	case "doctor":
		return "Run doctor"
	case "legacy":
		return "Use legacy"
	default:
		return "Back"
	}
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
			recommendation = []string{"Upstream требует модуль ядра AmneziaWG; текущая ОС или ядро его не поддерживает либо репозитории недоступны.", "Запустите doctor и проверьте версию ядра, архитектуру и доступность пакета/модуля amneziawg.", "На Fedora/RHEL-подобных системах повторите установку и разрешите добавить официальный COPR-репозиторий.", "Legacy и Upstream — разные сценарии; автоматической миграции между ними нет. При необходимости выберите engine legacy."}
		} else {
			recommendation = []string{"Upstream requires the AmneziaWG kernel module; this OS/kernel does not support it or the repositories are unavailable.", "Run doctor and check the kernel version, architecture, and availability of the amneziawg package/module.", "On Fedora/RHEL-like systems, retry and allow the official AmneziaWG COPR repository when prompted.", "If the provider cannot load the module, choose engine legacy for WireSock compatibility."}
		}
	} else if strings.Contains(strings.ToLower(detail), "docker: command not found") || strings.Contains(strings.ToLower(detail), "docker=missing") {
		if lang == langRU {
			recommendation = []string{"Docker Compose установлен, но команда docker недоступна на сервере.", "На Debian docker.io и docker-cli могут быть отдельными пакетами; установщик v2.4.2 проверяет и восстанавливает оба.", "Запустите doctor и проверьте пакет Docker, PATH и состояние docker.service.", "После исправления повторите install: конфигурация уже сохранена безопасно."}
		} else {
			recommendation = []string{"Docker Compose is installed, but the docker command is unavailable on the server.", "On Debian, docker.io and docker-cli may be separate packages; v2.4.2 checks and repairs both.", "Run doctor and check the Docker package, PATH, and docker.service status.", "After fixing Docker, retry install; the protected configuration is safe to reconcile."}
		}
	} else if strings.Contains(strings.ToLower(detail), "kernel-upgrade-failed") {
		if lang == langRU {
			recommendation = []string{"Автоматическое обновление ядра не завершилось.", "Запустите doctor и проверьте APT-репозитории, место на диске и ошибки package manager.", "Исправьте причину и повторите upstream; сервер автоматически не перезагружался.", "Если провайдер блокирует обновление ядра, используйте engine legacy или другой VDS."}
		} else {
			recommendation = []string{"The automatic kernel upgrade did not complete.", "Run doctor and check APT repositories, disk space, and package-manager errors.", "Fix the cause and retry upstream; the server was not rebooted automatically.", "If the provider blocks kernel upgrades, use engine legacy or another VDS."}
		}
	} else if strings.Contains(strings.ToLower(detail), "kernel-headers-unavailable-after-upgrade") {
		if lang == langRU {
			recommendation = []string{"После apt full-upgrade headers для текущего ядра всё ещё недоступны.", "Если установлен новый kernel, перезагрузите VDS вручную и затем повторите upstream.", "Автоматическая перезагрузка запрещена; сначала проверьте doctor после reboot.", "Если headers не публикуются провайдером, используйте engine legacy или другой VDS."}
		} else {
			recommendation = []string{"After apt full-upgrade, headers are still unavailable for the running kernel.", "If a new kernel was installed, reboot the VDS manually and retry upstream.", "Automatic reboot is disabled; run doctor after reboot.", "If the provider does not publish headers, use engine legacy or another VDS."}
		}
	} else if strings.Contains(strings.ToLower(detail), "kernel-headers-unavailable") || strings.Contains(strings.ToLower(detail), "no installation candidate") {
		if lang == langRU {
			recommendation = []string{"Для запущенного ядра нет доступного пакета linux-headers в текущих APT-источниках.", "Проверьте Ubuntu-репозитории и выполните apt full-upgrade; после установки нового ядра потребуется перезагрузка VDS.", "Не перезагружайте сервер автоматически: повторите upstream после reboot и проверьте doctor.", "Если провайдер предоставляет собственное ядро без headers, используйте engine legacy или другой VDS."}
		} else {
			recommendation = []string{"No linux-headers package is available for the running kernel in the configured APT sources.", "Check the Ubuntu repositories and run apt full-upgrade; a VDS reboot is required after installing a new kernel.", "The installer does not reboot automatically: retry upstream after reboot and run doctor.", "If the provider supplies a custom kernel without headers, use engine legacy or another VDS."}
		}
	} else if strings.Contains(strings.ToLower(detail), "module-load-failed") || strings.Contains(strings.ToLower(detail), "modprobe: fatal") {
		if lang == langRU {
			recommendation = []string{"Репозиторий найден, но модуль AmneziaWG не загрузился для текущего ядра.", "Проверьте linux-headers, dkms и результат: dkms status; dmesg | tail -50.", "Повторная установка обновлённой версии добавит headers/DKMS и попробует собрать модуль заново.", "Если headers или загрузка модулей запрещены провайдером, используйте engine legacy или другой VDS."}
		} else {
			recommendation = []string{"The repository was found, but the AmneziaWG module did not load for the running kernel.", "Check linux-headers, dkms, and run: dkms status; dmesg | tail -50.", "Retrying with the updated installer will add headers/DKMS and rebuild the module.", "If headers or module loading are blocked by the provider, use engine legacy or another VDS."}
		}
	} else if strings.Contains(strings.ToLower(detail), "ssh command timed out") || strings.Contains(strings.ToLower(detail), "context deadline exceeded") {
		if lang == langRU {
			recommendation = []string{"Долгая операция на сервере превысила SSH timeout; часто это apt full-upgrade, dnf install или сборка DKMS.", "Повторите установку с актуальным awg-vds: для install установлен timeout 30 минут.", "Если сервер медленный, используйте CLI-флаг --timeout 2400 и проверьте APT-lock после неудачи.", "Перезагрузка автоматически не выполняется; при необходимости используйте doctor после ручной проверки."}
		} else {
			recommendation = []string{"A long server operation exceeded the SSH timeout; this is often apt full-upgrade, dnf install, or a DKMS build.", "Retry with the current awg-vds; install now uses a 30-minute timeout.", "For a slow server, use --timeout 2400 and check for an APT lock after failure.", "No automatic reboot is performed; run doctor after manual inspection if needed."}
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

type uiProfile struct {
	Host       string `json:"host,omitempty"`
	User       string `json:"user,omitempty"`
	SSHPort    int    `json:"ssh_port,omitempty"`
	Identity   string `json:"identity_file,omitempty"`
	KnownHosts string `json:"known_hosts,omitempty"`
}

type uiProfileStore struct {
	Profiles map[string]uiProfile `json:"profiles"`
	Last     string               `json:"last,omitempty"`
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

func profilePath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "awg-vds", "last-profile.json"), nil
}

func loadUIProfile() uiProfile {
	store := loadUIProfileStore()
	return store.Profiles[store.Last]
}

func loadUIProfileStore() uiProfileStore {
	path, err := profilePath()
	if err != nil {
		return uiProfileStore{Profiles: map[string]uiProfile{}}
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return uiProfileStore{Profiles: map[string]uiProfile{}}
	}
	var store uiProfileStore
	if json.Unmarshal(b, &store) != nil || store.Profiles == nil {
		return uiProfileStore{Profiles: map[string]uiProfile{}}
	}
	return store
}

func selectUIProfile(in io.Reader, out io.Writer, lang uiLanguage) (uiProfile, string, error) {
	store := loadUIProfileStore()
	if len(store.Profiles) == 0 {
		return uiProfile{}, "", nil
	}
	names := make([]string, 0, len(store.Profiles))
	for name := range store.Profiles {
		names = append(names, name)
	}
	sort.Strings(names)
	choices := append([]string{"new connection"}, names...)
	selected := 0
	for i, name := range names {
		if name == store.Last {
			selected = i + 1
			break
		}
	}
	title := "Connection profile"
	if lang == langRU {
		title = "Профиль подключения"
		choices[0] = "новое подключение"
	}
	choice, err := dropdown(in, out, title, choices, selected)
	if err != nil {
		return uiProfile{}, "", err
	}
	if choice == "new connection" || choice == "новое подключение" {
		return uiProfile{}, "", nil
	}
	return store.Profiles[choice], choice, nil
}

func saveUIProfile(profile uiProfile) {
	name := profile.Host + "@" + profile.User
	saveUIProfileNamed(name, profile)
}

func saveUIProfileNamed(name string, profile uiProfile) {
	if name == "" {
		name = profile.Host + "@" + profile.User
	}
	path, err := profilePath()
	if err != nil {
		return
	}
	if os.MkdirAll(filepath.Dir(path), 0700) != nil {
		return
	}
	store := loadUIProfileStore()
	if store.Profiles == nil {
		store.Profiles = map[string]uiProfile{}
	}
	store.Profiles[name] = profile
	store.Last = name
	b, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(path, append(b, '\n'), 0600)
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
		if command[0] == "install" || command[0] == "update" || command[0] == "rotate-password" {
			confirmed, summaryErr := showSummaryTUI(in, out, selection.language, command)
			if summaryErr != nil {
				return summaryErr
			}
			if !confirmed {
				fmt.Fprintln(out, tr(selection.language, "cancelled"))
				continue
			}
		}
		password := ""
		if !commandHasIdentity(command) {
			password, err = ssh.ReadInteractivePassword(errOut)
			if err != nil {
				title, body := errorNotice(selection.language, selection.action, err)
				if noticeErr := showNoticeTUI(in, out, title, body); noticeErr != nil {
					return noticeErr
				}
				continue
			}
		}
		var operationErr error
		for {
			operationErr = runOperationTUI(in, out, command, selection.language, password)
			if operationErr == nil {
				break
			}
			title, body := errorNotice(selection.language, selection.action, operationErr)
			choice, noticeErr := showRecoveryNoticeTUI(in, out, selection.language, selection.action, operationErr, title, body)
			if noticeErr != nil {
				return noticeErr
			}
			switch choice {
			case "retry":
				continue
			case "doctor":
				command = commandAsDoctor(command)
				continue
			case "legacy":
				command = commandAsLegacy(command)
				continue
			}
			break
		}
		if operationErr == nil {
			fmt.Fprintln(out, tr(selection.language, "operation_completed"))
			if command[0] == "install" {
				panelPassword, passwordErr := readPanelPasswordTUI(context.Background(), command, password, errOut)
				if passwordErr != nil {
					if noticeErr := showNoticeTUI(in, out, tr(selection.language, "panel_password_unavailable"), tr(selection.language, "panel_password_unavailable")); noticeErr != nil {
						return noticeErr
					}
				} else {
					title := "Panel access"
					if selection.language == langRU {
						title = "Доступ к панели"
					}
					if noticeErr := showNoticeTUI(in, out, title, installAccessNotice(selection.language, command, panelPassword)); noticeErr != nil {
						return noticeErr
					}
				}
			}
			fmt.Fprintln(out, tr(selection.language, "return_menu"))
		}
	}
}

func commandHasIdentity(args []string) bool {
	for _, arg := range args {
		if arg == "--identity-file" {
			return true
		}
	}
	return false
}

func commandAsDoctor(args []string) []string {
	result := append([]string(nil), args...)
	if len(result) > 0 {
		result[0] = "doctor"
	}
	return result
}

func commandAsLegacy(args []string) []string {
	result := append([]string(nil), args...)
	for i := 0; i+1 < len(result); i++ {
		if result[i] == "--engine" {
			result[i+1] = "legacy"
			return result
		}
	}
	return result
}
