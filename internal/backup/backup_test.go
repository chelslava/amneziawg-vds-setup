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
	for _, want := range []string{"date -u", "tar", "/opt/awg-vds/backups", "chmod 600"} {
		if !strings.Contains(cmd, want) {
			t.Errorf("backup command lacks %q: %s", want, cmd)
		}
	}
}
