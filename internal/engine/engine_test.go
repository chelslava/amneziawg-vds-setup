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
		if strings.Contains(cmd, ":latest") || !strings.Contains(e.Image(), "@sha256:") || !strings.Contains(cmd, "--network host") {
			t.Fatalf("unsafe command for %s: %s", kind, cmd)
		}
	}
}

func TestHostOnlyUsesEnvFileAsWGHostSource(t *testing.T) {
	e, _ := Select(config.Legacy)
	s := state.State{Engine: config.Legacy, Image: e.Image(), Container: e.Container(), VPNPort: 1234, WebPort: 51821, TLSMode: "disabled", ConfigPath: "/opt/awg-vds/wireguard", BackupPath: "/opt/awg-vds/backups", Version: 1}
	cmd := e.UpdateCommand(s)
	if strings.Contains(cmd, "WG_HOST=") {
		t.Fatalf("engine command must not override WG_HOST: %s", cmd)
	}
	if !strings.Contains(cmd, "--env-file /opt/awg-vds/legacy.env") {
		t.Fatalf("engine command must use the generated env file: %s", cmd)
	}
}

func TestUpdateUsesCandidateImageAsRuntimeTarget(t *testing.T) {
	e, _ := Select(config.Legacy)
	s := state.State{Engine: config.Legacy, Image: "ghcr.io/example/previous:1", Container: e.Container(), Domain: "vpn.example.com", VPNPort: 1234, WebPort: 51821, TLSMode: "caddy", ConfigPath: "/opt/awg-vds/wireguard", BackupPath: "/opt/awg-vds/backups", Version: 1}
	cmd := e.UpdateCommand(s)
	if !strings.Contains(cmd, "ghcr.io/example/previous:1") || strings.Contains(cmd, e.Image()) {
		t.Fatalf("update command ignored candidate state image: %s", cmd)
	}
}

func TestInstallCommandsConfigureWireGuardForwarding(t *testing.T) {
	for _, kind := range []config.Engine{config.Legacy, config.Upstream} {
		e, _ := Select(kind)
		s := state.State{Engine: kind, Image: e.Image(), Container: e.Container(), VPNPort: 1234, WebPort: 51821, TLSMode: "disabled", ConfigPath: "/opt/awg-vds/wireguard", BackupPath: "/opt/awg-vds/backups", Version: 1}
		cmd := e.InstallCommand(s)
		for _, want := range []string{"ip link show wg0", "iptables -C FORWARD -i wg0", "--ctstate RELATED,ESTABLISHED", "iptables -t nat -C POSTROUTING -s 10.8.0.0/24", "-j MASQUERADE"} {
			if !strings.Contains(cmd, want) {
				t.Fatalf("%s install command lacks forwarding rule %q: %s", kind, want, cmd)
			}
		}
	}
}
