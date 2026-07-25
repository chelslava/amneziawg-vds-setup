package backup

import (
	"strings"
	"testing"

	"github.com/chelslava/amneziawg-vds-setup/v2/internal/config"
	"github.com/chelslava/amneziawg-vds-setup/v2/internal/state"
)

func TestBackupCommandIsTimestampedAndPrivate(t *testing.T) {
	s := state.State{Engine: config.Legacy, Container: "awg-vds-legacy", VPNPort: 1234, WebPort: 51821, TLSMode: "disabled", ConfigPath: "/opt/awg-vds/wireguard", BackupPath: "/opt/awg-vds/backups", Version: 1}
	cmd := Command(s)
	for _, want := range []string{"date -u", "tar", "/opt/awg-vds/backups", "chmod 600", "wireguard", "legacy.env", "upstream.env", "Caddyfile", "install-state.json", "panel-password", "sha256sum"} {
		if !strings.Contains(cmd, want) {
			t.Errorf("backup command lacks %q: %s", want, cmd)
		}
	}
}

func TestBackupResultAndRestoreCommand(t *testing.T) {
	path, checksum, err := ParseResult("BACKUP_PATH=/opt/awg-vds/backups/snapshot.tar.gz\nBACKUP_SHA256=abc123\n")
	if err != nil || path == "" || checksum != "abc123" {
		t.Fatalf("unexpected parsed backup metadata: %q %q %v", path, checksum, err)
	}
	if _, _, err := ParseResult("BACKUP_PATH=/tmp/no-checksum"); err == nil {
		t.Fatal("incomplete backup metadata was accepted")
	}
	cmd := RestoreCommand(path)
	if !strings.Contains(cmd, "tar -C /opt/awg-vds -xzf") || !strings.Contains(cmd, path) {
		t.Fatalf("restore command does not extract the selected snapshot: %s", cmd)
	}
}
