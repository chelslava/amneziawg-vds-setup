package backup

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/chelslava/amneziawg-vds-setup/v2/internal/state"
)

var safeName = regexp.MustCompile(`[^a-zA-Z0-9_.-]`)

func Command(s state.State) string {
	name := safeName.ReplaceAllString(s.Container, "-")
	return fmt.Sprintf("set -eu; install -d -m 700 %s; stamp=$(date -u +%%Y%%m%%dT%%H%%M%%SZ); out=%s/%s-$stamp.tar.gz; tar -C /opt/awg-vds --ignore-failed-read -czf \"$out\" wireguard legacy.env upstream.env Caddyfile caddy-data caddy-config install-state.json panel-password; chmod 600 \"$out\"; sha=$(sha256sum \"$out\" | awk '{print $1}'); printf 'BACKUP_PATH=%%s\\nBACKUP_SHA256=%%s\\n' \"$out\" \"$sha\"", shellQuote(s.BackupPath), shellQuote(s.BackupPath), name)
}

func RestoreCommand(archive string) string {
	return fmt.Sprintf("set -eu; test -s %s; install -d -m 700 /opt/awg-vds; tar -C /opt/awg-vds -xzf %s; printf 'RESTORED=%%s\\n' %s", shellQuote(archive), shellQuote(archive), shellQuote(archive))
}

func ParseResult(output string) (path, checksum string, err error) {
	for _, line := range strings.Split(output, "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			continue
		}
		switch key {
		case "BACKUP_PATH":
			path = value
		case "BACKUP_SHA256":
			checksum = value
		}
	}
	if path == "" || checksum == "" {
		return "", "", fmt.Errorf("backup command did not return path and checksum")
	}
	return path, checksum, nil
}

func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'" }
