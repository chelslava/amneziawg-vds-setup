package preflight

import (
	"strings"
	"testing"

	"github.com/chelslava/amneziawg-vds-setup/v2/internal/config"
)

func TestCommandGeneratesCorrectPortChecks(t *testing.T) {
	opts := config.Options{
		Engine:  config.Legacy,
		VPNPort: 1234,
		WebPort: 51821,
	}

	cmd := Command(opts)

	// TCP port check: awk '{print $4}'
	expectedTCP := "if ss -ltnH | awk '{print $4}' | grep -Eq '(^|:)51821$'; then printf 'PORT_TCP_51821=busy\\n'; else printf 'PORT_TCP_51821=free\\n'; fi;"
	if !strings.Contains(cmd, expectedTCP) {
		t.Errorf("command does not contain expected TCP check: %q\ngot:\n%s", expectedTCP, cmd)
	}

	// UDP port check: awk '{print $4}' (local address:port column in ss -lunH, not $5 which is peer)
	expectedUDP := "if ss -lunH | awk '{print $4}' | grep -Eq '(^|:)1234$'; then printf 'PORT_UDP_1234=busy\\n'; else printf 'PORT_UDP_1234=free\\n'; fi;"
	if !strings.Contains(cmd, expectedUDP) {
		t.Errorf("command does not contain expected UDP check: %q\ngot:\n%s", expectedUDP, cmd)
	}

	// Ensure awk '{print $5}' is NOT present for ss -lunH
	if strings.Contains(cmd, "ss -lunH | awk '{print $5}'") {
		t.Errorf("command contains incorrect column index $5 for ss -lunH: %s", cmd)
	}
}

func TestCommandCustomPorts(t *testing.T) {
	opts := config.Options{
		Engine:  config.Legacy,
		VPNPort: 51820,
		WebPort: 8080,
	}

	cmd := Command(opts)

	expectedTCP := "PORT_TCP_8080=busy"
	if !strings.Contains(cmd, expectedTCP) {
		t.Errorf("expected %q in command, got %s", expectedTCP, cmd)
	}

	expectedUDP := "PORT_UDP_51820=busy"
	if !strings.Contains(cmd, expectedUDP) {
		t.Errorf("expected %q in command, got %s", expectedUDP, cmd)
	}
}
