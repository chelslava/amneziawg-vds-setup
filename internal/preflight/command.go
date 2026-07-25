package preflight

import (
	"fmt"
	"strings"

	"github.com/chelslava/amneziawg-vds-setup/v2/internal/config"
)

func Command(o config.Options) string {
	var b strings.Builder
	b.WriteString("set -eu; . /etc/os-release; printf 'OS=%s %s\\n' \"$ID\" \"$VERSION_ID\"; arch=$(uname -m); printf 'ARCH=%s\\n' \"$arch\"; case \"$ID\" in ubuntu|debian) ;; *) printf 'ERROR=Unsupported OS: %s\\n' \"$ID\" >&2; exit 1;; esac; ")
	if o.Engine == config.Legacy {
		b.WriteString("case \"$arch\" in x86_64|amd64) ;; *) printf 'ERROR=Legacy engine supports linux/amd64 only; detected %s\\n' \"$arch\" >&2; exit 1;; esac; ")
	}
	b.WriteString("command -v docker >/dev/null || printf 'WARNING=Docker is not installed\\n'; docker compose version >/dev/null 2>&1 || printf 'WARNING=Docker Compose v2 is not installed\\n'; ")
	b.WriteString("df -Pk / | awk 'NR==2 {printf \"DISK_MB=%d\\n\", $4/1024}'; free -m | awk '/^Mem:/ {printf \"MEM_MB=%d\\n\", $7}'; ")
	fmt.Fprintf(&b, "if ss -ltnH | awk '{print $4}' | grep -Eq '(^|:)%d$'; then printf 'PORT_TCP_%d=busy\\n'; else printf 'PORT_TCP_%d=free\\n'; fi; if ss -lunH | awk '{print $5}' | grep -Eq '(^|:)%d$'; then printf 'PORT_UDP_%d=busy\\n'; else printf 'PORT_UDP_%d=free\\n'; fi; ", o.WebPort, o.WebPort, o.WebPort, o.VPNPort, o.VPNPort, o.VPNPort)
	b.WriteString("if command -v ufw >/dev/null 2>&1; then ufw status | head -1 | tr ' ' '_' | sed 's/^/FIREWALL=/'; elif command -v nft >/dev/null 2>&1; then printf 'FIREWALL=nftables\\n'; else printf 'FIREWALL=unknown\\n'; fi; ")
	if o.Domain != "" {
		fmt.Fprintf(&b, "getent hosts %s >/dev/null 2>&1 && printf 'DNS=ok\\n' || printf 'DNS=unresolved\\n'; ", quote(o.Domain))
	}
	if o.Engine == config.Upstream {
		b.WriteString("if test -e /sys/module/amneziawg || command -v awg >/dev/null 2>&1; then printf 'AMNEZIAWG=present\\n'; elif apt-cache policy amneziawg 2>/dev/null | grep -q 'Candidate: [^()]'; then printf 'AMNEZIAWG=installable\\n'; else printf 'AMNEZIAWG=unsupported\\n'; fi; ")
	}
	b.WriteString("printf 'PREFLIGHT=ok\\n'")
	return b.String()
}
func quote(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'" }
