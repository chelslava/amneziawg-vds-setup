package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

func interactiveCommand(in *bufio.Reader, out io.Writer, choice string) ([]string, error) {
	return interactiveCommandLanguageMode(in, out, choice, langEN, false)
}

func interactiveCommandLanguage(in *bufio.Reader, out io.Writer, choice string, lang uiLanguage) ([]string, error) {
	return interactiveCommandLanguageMode(in, out, choice, lang, false)
}

func interactiveCommandTUI(in *bufio.Reader, out io.Writer, choice string, lang uiLanguage) ([]string, error) {
	return interactiveCommandLanguageMode(in, out, choice, lang, true)
}

func interactiveCommandLanguageMode(in *bufio.Reader, out io.Writer, choice string, lang uiLanguage, useDropdown bool) ([]string, error) {
	command := choice
	if mapped, ok := map[string]string{"1": "install", "2": "status", "3": "doctor", "4": "update", "5": "backup", "6": "rotate-password"}[choice]; ok {
		command = mapped
	}
	if command != "install" && command != "status" && command != "doctor" && command != "update" && command != "backup" && command != "rotate-password" {
		command = ""
	}
	if command == "" {
		return nil, nil
	}
	args := []string{command}
	connection, err := interactiveConnectionLanguageMode(in, out, lang, useDropdown)
	if err != nil {
		return nil, err
	}
	args = append(args, connection...)
	if command == "install" || command == "doctor" {
		engine, err := selectInteractiveValue(in, out, tr(lang, "engine"), []string{"legacy", "upstream"}, 0, useDropdown)
		if err != nil {
			return nil, err
		}
		args = append(args, "--engine", engine)
	}
	if command == "install" {
		vpn, err := promptInt(in, out, tr(lang, "vpn_port"), 1234)
		if err != nil {
			return nil, err
		}
		web, err := promptInt(in, out, tr(lang, "panel_port"), 51821)
		if err != nil {
			return nil, err
		}
		args = append(args, "--vpn-port", strconv.Itoa(vpn), "--web-port", strconv.Itoa(web))
		domain, err := prompt(in, out, tr(lang, "domain"), "")
		if err != nil {
			return nil, err
		}
		if domain != "" {
			args = append(args, "--domain", domain)
			tlsChoice, err := selectInteractiveValue(in, out, tr(lang, "enable_tls"), []string{"no", "yes"}, 0, useDropdown)
			if err != nil {
				return nil, err
			}
			if tlsChoice == "yes" {
				args = append(args, "--tls")
			}
		}
		restrict, err := prompt(in, out, tr(lang, "panel_restriction"), "")
		if err != nil {
			return nil, err
		}
		if restrict != "" {
			args = append(args, "--restrict-panel-ip", restrict)
		}
	}
	return args, nil
}

func selectInteractiveValue(in *bufio.Reader, out io.Writer, label string, choices []string, selected int, useDropdown bool) (string, error) {
	if useDropdown {
		return dropdown(in, out, label, choices, selected)
	}
	return promptChoice(in, out, label, choices, choices[selected])
}

func interactiveConnection(in *bufio.Reader, out io.Writer) ([]string, error) {
	return interactiveConnectionLanguage(in, out, langEN)
}

func interactiveConnectionLanguage(in *bufio.Reader, out io.Writer, lang uiLanguage) ([]string, error) {
	return interactiveConnectionLanguageMode(in, out, lang, false)
}

func interactiveConnectionLanguageMode(in *bufio.Reader, out io.Writer, lang uiLanguage, useDropdown bool) ([]string, error) {
	host, err := prompt(in, out, tr(lang, "vds_address"), "")
	if err != nil {
		return nil, err
	}
	if host == "" {
		return nil, errors.New(tr(lang, "vds_required"))
	}
	user, err := prompt(in, out, tr(lang, "ssh_user"), "root")
	if err != nil {
		return nil, err
	}
	port, err := promptInt(in, out, tr(lang, "ssh_port"), 22)
	if err != nil {
		return nil, err
	}
	authMethod, err := selectInteractiveValue(in, out, tr(lang, "ssh_auth"), []string{"key", "password"}, 0, useDropdown)
	if err != nil {
		return nil, err
	}
	identity := ""
	if authMethod == "key" {
		identity, err = prompt(in, out, tr(lang, "identity_file"), "")
		if err != nil {
			return nil, err
		}
	}
	knownHosts, err := prompt(in, out, tr(lang, "known_hosts"), "")
	if err != nil {
		return nil, err
	}
	args := []string{"--host", host, "--user", user, "--ssh-port", strconv.Itoa(port)}
	if identity != "" {
		args = append(args, "--identity-file", identity)
	} else {
		fmt.Fprintln(out, tr(lang, "password_notice"))
	}
	if knownHosts != "" {
		args = append(args, "--known-hosts", knownHosts)
	}
	return args, nil
}

func prompt(in *bufio.Reader, out io.Writer, label, defaultValue string) (string, error) {
	if defaultValue != "" {
		fmt.Fprintf(out, "%s [%s]: ", label, defaultValue)
	} else {
		fmt.Fprintf(out, "%s: ", label)
	}
	line, err := in.ReadString('\n')
	if err != nil && len(line) == 0 {
		return "", err
	}
	line = strings.TrimSpace(line)
	if strings.EqualFold(line, "back") || strings.EqualFold(line, "назад") {
		return "", errTUIBack
	}
	if line == "" {
		return defaultValue, nil
	}
	return line, nil
}

func promptInt(in *bufio.Reader, out io.Writer, label string, defaultValue int) (int, error) {
	value, err := prompt(in, out, label, strconv.Itoa(defaultValue))
	if err != nil {
		return 0, err
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 1 || parsed > 65535 {
		return 0, fmt.Errorf("%s must be an integer from 1 to 65535", label)
	}
	return parsed, nil
}

func promptChoice(in *bufio.Reader, out io.Writer, label string, choices []string, defaultValue string) (string, error) {
	fmt.Fprintf(out, "%s (%s)\n", label, strings.Join(choices, "/"))
	value, err := prompt(in, out, "Choice", defaultValue)
	if err != nil {
		return "", err
	}
	for _, choice := range choices {
		if value == choice {
			return value, nil
		}
	}
	return "", fmt.Errorf("choice must be one of %s", strings.Join(choices, ", "))
}

func confirm(in *bufio.Reader, out io.Writer, label string) bool {
	value, err := prompt(in, out, label+"? type yes", "no")
	return err == nil && strings.EqualFold(value, "yes")
}

func tr(lang uiLanguage, key string) string {
	if lang == langRU {
		if value, ok := map[string]string{
			"select_action": "Выберите действие", "goodbye": "До свидания.", "invalid_menu": "Выберите число от 0 до 7.", "cancelled": "Отменено.", "operation_failed": "Операция завершилась ошибкой:", "operation_completed": "Операция завершена.", "return_menu": "Возврат в главное меню…", "proceed": "Продолжить", "engine": "Движок", "vpn_port": "UDP-порт VPN", "panel_port": "TCP-порт панели", "domain": "Домен (пусто = HTTP по адресу VDS)", "enable_tls": "Включить TLS через Caddy", "panel_restriction": "Ограничение панели по IP (необязательно)", "vds_address": "Адрес VDS", "vds_required": "адрес VDS обязателен", "ssh_user": "Пользователь SSH", "ssh_port": "Порт SSH", "ssh_auth": "Метод SSH-аутентификации", "identity_file": "Файл SSH-ключа", "known_hosts": "Файл known_hosts (пусто = системный)", "password_notice": "Пароль запросит OpenSSH; awg-vds его не читает и не сохраняет.",
		}[key]; ok {
			return value
		}
	}
	return map[string]string{
		"select_action": "Select an action", "goodbye": "Goodbye.", "invalid_menu": "Choose a number from 0 to 7.", "cancelled": "Cancelled.", "operation_failed": "Operation failed:", "operation_completed": "Operation completed.", "return_menu": "Returning to main menu…", "proceed": "Proceed", "engine": "Engine", "vpn_port": "VPN UDP port", "panel_port": "Panel TCP port", "domain": "Domain (empty for host-only HTTP)", "enable_tls": "Enable TLS with Caddy", "panel_restriction": "Optional panel IP restriction", "vds_address": "VDS address", "vds_required": "VDS address is required", "ssh_user": "SSH user", "ssh_port": "SSH port", "ssh_auth": "SSH auth method", "identity_file": "SSH identity file", "known_hosts": "known_hosts file (empty for system default)", "password_notice": "SSH password will be requested interactively by OpenSSH; it is not handled or stored by awg-vds.",
	}[key]
}
