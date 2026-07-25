package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// interactiveMenu is deliberately line-oriented: it works in Windows Terminal,
// classic PowerShell, macOS Terminal, Linux SSH sessions, and redirected stdin.
// The flag interface remains available for automation and CI.
func interactiveMenu(in *bufio.Reader, out, errOut io.Writer) error {
	for {
		fmt.Fprintln(out, "")
		fmt.Fprintln(out, "╭────────────────────────────────────────────╮")
		fmt.Fprintln(out, "│ awg-vds 2 · AmneziaWG VDS control center   │")
		fmt.Fprintln(out, "╰────────────────────────────────────────────╯")
		fmt.Fprintln(out, "  1  Install / reconcile")
		fmt.Fprintln(out, "  2  Status")
		fmt.Fprintln(out, "  3  Doctor")
		fmt.Fprintln(out, "  4  Update (backup + rollback)")
		fmt.Fprintln(out, "  5  Backup")
		fmt.Fprintln(out, "  6  Rotate panel password")
		fmt.Fprintln(out, "  7  CLI help")
		fmt.Fprintln(out, "  0  Exit")
		choice, err := prompt(in, out, "Select an action", "0")
		if err != nil {
			return err
		}
		if choice == "0" {
			fmt.Fprintln(out, "Goodbye.")
			return nil
		}
		if choice == "7" {
			usage(out)
			continue
		}
		command, err := interactiveCommand(in, out, choice)
		if err != nil {
			fmt.Fprintln(out, "Input error:", err)
			continue
		}
		if command == nil {
			fmt.Fprintln(out, "Choose a number from 0 to 7.")
			continue
		}
		if (command[0] == "install" || command[0] == "update" || command[0] == "rotate-password") && !confirm(in, out, "Proceed") {
			fmt.Fprintln(out, "Cancelled.")
			continue
		}
		fmt.Fprintln(out, "")
		if err := runCommand(command, out, errOut); err != nil {
			fmt.Fprintln(out, "Operation failed:", err)
		} else {
			fmt.Fprintln(out, "Operation completed.")
		}
		if !confirm(in, out, "Return to menu") {
			return nil
		}
	}
}

func interactiveCommand(in *bufio.Reader, out io.Writer, choice string) ([]string, error) {
	command := map[string]string{"1": "install", "2": "status", "3": "doctor", "4": "update", "5": "backup", "6": "rotate-password"}[choice]
	if command == "" {
		return nil, nil
	}
	args := []string{command}
	connection, err := interactiveConnection(in, out)
	if err != nil {
		return nil, err
	}
	args = append(args, connection...)
	if command == "install" || command == "doctor" {
		engine, err := promptChoice(in, out, "Engine", []string{"legacy", "upstream"}, "legacy")
		if err != nil {
			return nil, err
		}
		args = append(args, "--engine", engine)
	}
	if command == "install" {
		vpn, err := promptInt(in, out, "VPN UDP port", 1234)
		if err != nil {
			return nil, err
		}
		web, err := promptInt(in, out, "Panel TCP port", 51821)
		if err != nil {
			return nil, err
		}
		args = append(args, "--vpn-port", strconv.Itoa(vpn), "--web-port", strconv.Itoa(web))
		domain, err := prompt(in, out, "Domain (empty for host-only HTTP)", "")
		if err != nil {
			return nil, err
		}
		if domain != "" {
			args = append(args, "--domain", domain)
			if confirm(in, out, "Enable TLS with Caddy") {
				args = append(args, "--tls")
			}
		}
		restrict, err := prompt(in, out, "Optional panel IP restriction", "")
		if err != nil {
			return nil, err
		}
		if restrict != "" {
			args = append(args, "--restrict-panel-ip", restrict)
		}
	}
	return args, nil
}

func interactiveConnection(in *bufio.Reader, out io.Writer) ([]string, error) {
	host, err := prompt(in, out, "VDS address", "")
	if err != nil {
		return nil, err
	}
	if host == "" {
		return nil, errors.New("VDS address is required")
	}
	user, err := prompt(in, out, "SSH user", "root")
	if err != nil {
		return nil, err
	}
	port, err := promptInt(in, out, "SSH port", 22)
	if err != nil {
		return nil, err
	}
	identity, err := prompt(in, out, "SSH identity file (empty for interactive SSH auth)", "")
	if err != nil {
		return nil, err
	}
	knownHosts, err := prompt(in, out, "known_hosts file (empty for system default)", "")
	if err != nil {
		return nil, err
	}
	args := []string{"--host", host, "--user", user, "--ssh-port", strconv.Itoa(port)}
	if identity != "" {
		args = append(args, "--identity-file", identity)
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
