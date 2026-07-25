package engine

import (
	"strings"
	"testing"

	"github.com/chelslava/amneziawg-vds-setup/v2/internal/config"
	"github.com/chelslava/amneziawg-vds-setup/v2/internal/state"
)

func TestSelectEngine(t *testing.T) {
	for _, kind := range []config.Engine{config.Legacy, config.Upstream} {
		e, err := Select(kind)
		if err != nil {
			t.Fatal(err)
		}
		if e.Name() != kind {
			t.Fatalf("selected %s for %s", e.Name(), kind)
		}
	}
	if _, err := Select("invalid"); err == nil {
		t.Fatal("invalid engine accepted")
	}
}

func TestPinnedImagesAndHostNetwork(t *testing.T) {
	for _, kind := range []config.Engine{config.Legacy, config.Upstream} {
		e, _ := Select(kind)
		s := state.State{Engine: kind, Image: e.Image(), Container: e.Container(), Domain: "vpn.example", VPNPort: 1234, WebPort: 51821, TLSMode: "disabled", ConfigPath: "/opt/awg-vds/wireguard", BackupPath: "/opt/awg-vds/backups", Version: 1}
		cmd := e.UpdateCommand(s)
		if strings.Contains(cmd, ":latest") || !strings.Contains(cmd, "--network host") {
			t.Fatalf("unsafe command for %s: %s", kind, cmd)
		}
	}
}
