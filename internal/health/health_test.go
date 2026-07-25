package health

import (
	"strings"
	"testing"

	"github.com/chelslava/amneziawg-vds-setup/v2/internal/config"
	"github.com/chelslava/amneziawg-vds-setup/v2/internal/state"
)

func TestHealthChecksRuntimeImageAgainstState(t *testing.T) {
	s := state.State{Version: 1, Engine: config.Legacy, Image: "ghcr.io/example/awg:1.0.0", Container: "awg-vds-legacy", VPNPort: 1234, WebPort: 51821, TLSMode: "disabled", ConfigPath: "/opt/awg-vds/wireguard", BackupPath: "/opt/awg-vds/backups"}
	cmd := Command(s)
	if !strings.Contains(cmd, "{{.Config.Image}}") || !strings.Contains(cmd, "ghcr.io/example/awg:1.0.0") {
		t.Fatalf("health command does not verify the persisted image: %s", cmd)
	}
}
