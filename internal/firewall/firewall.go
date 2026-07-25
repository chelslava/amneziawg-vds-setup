package firewall

import (
	"fmt"
	"strings"
)

func Command(vpnPort, webPort int, domain, restrictIP string) string {
	var b strings.Builder
	b.WriteString("set -eu; ")
	b.WriteString("if command -v ufw >/dev/null 2>&1 && ufw status | grep -q '^Status: active'; then ")
	fmt.Fprintf(&b, "ufw allow %d/udp >/dev/null; ", vpnPort)
	if domain != "" {
		b.WriteString("ufw allow 80/tcp >/dev/null; ufw allow 443/tcp >/dev/null; ")
		if restrictIP != "" {
			fmt.Fprintf(&b, "ufw allow from %s to any port %d proto tcp >/dev/null; ufw deny %d/tcp >/dev/null || true; ", restrictIP, webPort, webPort)
		} else {
			fmt.Fprintf(&b, "ufw allow %d/tcp >/dev/null; ", webPort)
		}
	} else {
		fmt.Fprintf(&b, "ufw allow %d/tcp >/dev/null; ", webPort)
	}
	b.WriteString("printf 'FIREWALL=ufw\\n'; else printf 'FIREWALL=not-configured\\n'; fi")
	return b.String()
}
