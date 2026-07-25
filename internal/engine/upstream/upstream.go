package upstream

import (
	"fmt"
	"strings"

	"github.com/chelslava/amneziawg-vds-setup/v2/internal/config"
	"github.com/chelslava/amneziawg-vds-setup/v2/internal/engine"
	"github.com/chelslava/amneziawg-vds-setup/v2/internal/state"
)

const Image = "ghcr.io/wg-easy/wg-easy@sha256:4ffc03c35dce5456bbb2fa6b136a1eeb196394548dee0650ae692efdd1062e01"

type Engine struct{}

func (Engine) Name() config.Engine                   { return config.Upstream }
func (Engine) Image() string                         { return Image }
func (Engine) Container() string                     { return "awg-vds-upstream" }
func (e Engine) InstallCommand(s state.State) string { return runCommand(s, true) }
func (e Engine) UpdateCommand(s state.State) string  { return runCommand(s, false) }
func (e Engine) StatusCommand(s state.State) string {
	return fmt.Sprintf("docker inspect -f '{{.State.Status}}' %s", quote(s.Container))
}

func runCommand(s state.State, first bool) string {
	install := ""
	if first {
		install = "install -d -m 700 /opt/awg-vds/wireguard /opt/awg-vds; printf '%s\\n' net.ipv4.ip_forward=1 net.ipv4.conf.all.src_valid_mark=1 > /etc/sysctl.d/99-amneziawg-v2.conf; sysctl --system >/dev/null; "
	}
	return install + fmt.Sprintf("docker pull %s; docker rm -f %s >/dev/null 2>&1 || true; docker run -d --name %s --network host --env WG_HOST=%s --env PORT=%d --env WG_PORT=%d --env WG_PERSISTENT_KEEPALIVE=25 --env WG_DEFAULT_DNS=1.1.1.1,1.0.0.0 --env EXPERIMENTAL_AWG=true --env OVERRIDE_AUTO_AWG=awg --env-file /opt/awg-vds/upstream.env -v /opt/awg-vds/wireguard:/etc/wireguard --cap-add=NET_ADMIN --cap-add=SYS_MODULE --device /dev/net/tun:/dev/net/tun --restart unless-stopped %s", quote(Image), quote(s.Container), quote(s.Container), quote(s.Domain), s.WebPort, s.VPNPort, quote(Image))
}
func quote(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'" }

var _ engine.Engine = Engine{}
