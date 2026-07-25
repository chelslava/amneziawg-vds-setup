package engine

import (
	"fmt"
	"strings"

	"github.com/chelslava/amneziawg-vds-setup/v2/internal/config"
	"github.com/chelslava/amneziawg-vds-setup/v2/internal/state"
)

type legacyEngine struct{}

func (legacyEngine) Name() config.Engine                   { return config.Legacy }
func (legacyEngine) Image() string                         { return "ghcr.io/yokitoki/awg-easy:1.0.1" }
func (legacyEngine) Container() string                     { return "awg-vds-legacy" }
func (e legacyEngine) InstallCommand(s state.State) string { return engineCommand(e, s, true, false) }
func (e legacyEngine) UpdateCommand(s state.State) string  { return engineCommand(e, s, false, false) }
func (e legacyEngine) StatusCommand(s state.State) string {
	return fmt.Sprintf("docker inspect -f '{{.State.Status}}' %s", quoteEngine(s.Container))
}

type upstreamEngine struct{}

func (upstreamEngine) Name() config.Engine                   { return config.Upstream }
func (upstreamEngine) Image() string                         { return "ghcr.io/wg-easy/wg-easy:15.2.1" }
func (upstreamEngine) Container() string                     { return "awg-vds-upstream" }
func (e upstreamEngine) InstallCommand(s state.State) string { return engineCommand(e, s, true, true) }
func (e upstreamEngine) UpdateCommand(s state.State) string  { return engineCommand(e, s, false, true) }
func (e upstreamEngine) StatusCommand(s state.State) string {
	return fmt.Sprintf("docker inspect -f '{{.State.Status}}' %s", quoteEngine(s.Container))
}

type engineInfo interface {
	Image() string
	Container() string
}

func engineCommand(e engineInfo, s state.State, first, upstream bool) string {
	init := ""
	if first {
		init = "install -d -m 700 /opt/awg-vds/wireguard /opt/awg-vds; printf '%s\\n' net.ipv4.ip_forward=1 net.ipv4.conf.all.src_valid_mark=1 > /etc/sysctl.d/99-amneziawg-v2.conf; sysctl --system >/dev/null; "
	}
	extra := "--env EXPERIMENTAL_AWG=false --env-file /opt/awg-vds/legacy.env -v /opt/awg-vds/wireguard:/etc/amnezia/amneziawg -v /opt/awg-vds/wireguard:/etc/wireguard"
	if upstream {
		extra = "--env EXPERIMENTAL_AWG=true --env OVERRIDE_AUTO_AWG=awg --env-file /opt/awg-vds/upstream.env -v /opt/awg-vds/wireguard:/etc/wireguard"
	}
	return init + fmt.Sprintf("docker pull %s; docker rm -f %s >/dev/null 2>&1 || true; docker run -d --name %s --network host --env WG_HOST=%s --env PORT=%d --env WG_PORT=%d --env WG_PERSISTENT_KEEPALIVE=25 --env WG_DEFAULT_DNS=1.1.1.1,1.0.0.1 %s --cap-add=NET_ADMIN --cap-add=SYS_MODULE --device /dev/net/tun:/dev/net/tun --restart unless-stopped %s", quoteEngine(e.Image()), quoteEngine(e.Container()), quoteEngine(e.Container()), quoteEngine(s.Domain), s.WebPort, s.VPNPort, extra, quoteEngine(e.Image()))
}
func quoteEngine(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'" }
