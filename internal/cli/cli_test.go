package cli

import (
	"strings"
	"testing"

	"github.com/chelslava/amneziawg-vds-setup/v2/internal/config"
	"github.com/chelslava/amneziawg-vds-setup/v2/internal/state"
)

func TestParseInstallArguments(t *testing.T) {
	o, _, err := parse([]string{"install", "--host", "vpn.example", "--ssh-port", "2222", "--user", "admin", "--engine", "upstream", "--vpn-port", "443", "--web-port", "8443", "--domain", "vpn.example", "--tls"})
	if err != nil {
		t.Fatal(err)
	}
	if o.Host != "vpn.example" || o.SSHPort != 2222 || o.User != "admin" || o.Engine != config.Upstream || o.VPNPort != 443 || o.WebPort != 8443 || !o.TLS {
		t.Fatalf("unexpected options: %+v", o)
	}
}

func TestPasswordArgumentsRejected(t *testing.T) {
	if !containsSecretFlag([]string{"install", "--host", "x", "--password=secret"}) {
		t.Fatal("password argument was not rejected")
	}
	if containsSecretFlag([]string{"install", "--host", "x", "--identity-file", "id_ed25519"}) {
		t.Fatal("identity flag was incorrectly rejected")
	}
}

func TestAutomaticMigrationRejected(t *testing.T) {
	s := state.State{Engine: config.Legacy}
	if err := ValidateEngineReuse(s, config.Upstream); err == nil || !strings.Contains(err.Error(), "automatic migration") {
		t.Fatalf("unexpected error: %v", err)
	}
}
