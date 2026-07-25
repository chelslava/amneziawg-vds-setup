package tls

import (
	"fmt"
	"strings"
)

const Image = "caddy:2.9.1-alpine"

func Command(domain string, webPort int) string {
	if domain == "" {
		return "docker rm -f awg-vds-caddy >/dev/null 2>&1 || true"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "set -eu; install -d -m 700 /opt/awg-vds/caddy-data /opt/awg-vds/caddy-config; printf '%%s {\\n reverse_proxy 127.0.0.1:%d\\n}\\n' %s > /opt/awg-vds/Caddyfile; chmod 600 /opt/awg-vds/Caddyfile; docker pull %s; docker rm -f awg-vds-caddy >/dev/null 2>&1 || true; docker run -d --name awg-vds-caddy --network host -v /opt/awg-vds/Caddyfile:/etc/caddy/Caddyfile:ro -v /opt/awg-vds/caddy-data:/data -v /opt/awg-vds/caddy-config:/config --restart unless-stopped %s >/dev/null", webPort, shellQuote(domain), Image, Image)
	return b.String()
}

func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'" }
