package firewall

import (
	"fmt"
	"strings"
)

func Command(vpnPort, webPort int, tlsEnabled bool, restrictIP string) string {
	var b strings.Builder
	b.WriteString("set -eu; ")
	b.WriteString("if command -v ufw >/dev/null 2>&1 && ufw status | grep -q '^Status: active'; then ")
	fmt.Fprintf(&b, "ufw allow %d/udp >/dev/null; ", vpnPort)
	if tlsEnabled {
		b.WriteString("ufw allow 80/tcp >/dev/null; ufw allow 443/tcp >/dev/null; ")
		fmt.Fprintf(&b, "ufw delete allow %d/tcp >/dev/null 2>&1 || true; ufw delete deny %d/tcp >/dev/null 2>&1 || true; ", webPort, webPort)
		if restrictIP != "" {
			fmt.Fprintf(&b, "ufw insert 1 allow from %s to any port %d proto tcp >/dev/null; ufw insert 2 deny %d/tcp >/dev/null; ", restrictIP, webPort, webPort)
		} else {
			fmt.Fprintf(&b, "ufw deny %d/tcp >/dev/null; ", webPort)
		}
	} else {
		fmt.Fprintf(&b, "ufw allow %d/tcp >/dev/null; ", webPort)
	}
	b.WriteString("printf 'FIREWALL=ufw\\n'; else printf 'FIREWALL=not-configured\\n'; fi")
	return b.String()
}
