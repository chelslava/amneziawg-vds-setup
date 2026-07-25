package tls

import (
	"fmt"
	"strings"
)

const Image = "caddy@sha256:b4e3952384eb9524a887633ce65c752dd7c71314d2c2acf98cd5c715aaa534f0"

func Command(enabled bool, domain string, webPort int) string {
	if !enabled {
		return "docker rm -f awg-vds-caddy >/dev/null 2>&1 || true"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "set -eu; install -d -m 700 /opt/awg-vds/caddy-data /opt/awg-vds/caddy-config; printf '%%s {\\n reverse_proxy 127.0.0.1:%d\\n}\\n' %s > /opt/awg-vds/Caddyfile; chmod 600 /opt/awg-vds/Caddyfile; docker pull %s; docker rm -f awg-vds-caddy >/dev/null 2>&1 || true; docker run -d --name awg-vds-caddy --network host -v /opt/awg-vds/Caddyfile:/etc/caddy/Caddyfile:ro -v /opt/awg-vds/caddy-data:/data -v /opt/awg-vds/caddy-config:/config --restart unless-stopped %s >/dev/null", webPort, shellQuote(domain), Image, Image)
	return b.String()
}

func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'" }
