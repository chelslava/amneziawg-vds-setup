package firewall

import (
	"strings"
	"testing"
)

func TestTLSFirewallDoesNotOpenBackendPanelByDefault(t *testing.T) {
	cmd := Command(1234, 51821, true, "")
	if !strings.Contains(cmd, "ufw allow 80/tcp") || !strings.Contains(cmd, "ufw allow 443/tcp") {
		t.Fatalf("TLS ports are missing: %s", cmd)
	}
	if !strings.Contains(cmd, "ufw deny 51821/tcp") || strings.Contains(cmd, "ufw allow 51821/tcp >/dev/null") {
		t.Fatalf("backend panel is not closed in TLS mode: %s", cmd)
	}
}

func TestPlainHTTPFirewallOpensConfiguredPanel(t *testing.T) {
	cmd := Command(1234, 51821, false, "")
	if !strings.Contains(cmd, "ufw allow 51821/tcp") || strings.Contains(cmd, "ufw allow 443/tcp") {
		t.Fatalf("unexpected plain HTTP firewall policy: %s", cmd)
	}
}

func TestTLSFirewallRestrictsBackendPanel(t *testing.T) {
	cmd := Command(1234, 51821, true, "198.51.100.7")
	if !strings.Contains(cmd, "ufw insert 1 allow from 198.51.100.7") || !strings.Contains(cmd, "ufw insert 2 deny 51821/tcp") {
		t.Fatalf("restricted TLS policy is incomplete: %s", cmd)
	}
}

func TestFirewalldPolicyIsIdempotentAndKeepsTLSBackendPrivate(t *testing.T) {
	cmd := Command(1234, 51821, true, "198.51.100.7")
	for _, want := range []string{"firewall-cmd --permanent --add-port=1234/udp", "--add-service=http", "--add-service=https", "--remove-port=51821/tcp", "--reload", "FIREWALL=firewalld"} {
		if !strings.Contains(cmd, want) {
			t.Fatalf("firewalld policy lacks %q: %s", want, cmd)
		}
	}
}
