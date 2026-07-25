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
	return fmt.Sprintf("set -eu; install -d -m 700 %s; stamp=$(date -u +%%Y%%m%%dT%%H%%M%%SZ); out=%s/%s-$stamp.tar.gz; tar -C %s -czf \"$out\" %s; chmod 600 \"$out\"; printf 'BACKUP_PATH=%%s\\n' \"$out\"", shellQuote(s.BackupPath), shellQuote(s.BackupPath), name, shellQuote(parent(s.ConfigPath)), shellQuote(base(s.ConfigPath)))
}

func parent(path string) string {
	i := strings.LastIndex(path, "/")
	if i < 1 {
		return "/"
	}
	return path[:i]
}
func base(path string) string {
	i := strings.LastIndex(path, "/")
	if i < 0 {
		return path
	}
	return path[i+1:]
}
func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'" }
