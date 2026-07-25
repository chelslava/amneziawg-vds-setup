package health

import (
	"fmt"
	"strings"

	"github.com/chelslava/amneziawg-vds-setup/v2/internal/state"
)

func Command(s state.State) string {
	return fmt.Sprintf("set -eu; docker inspect -f '{{.State.Status}}' %s | grep -qx running; docker inspect -f '{{.Config.Image}}' %s | grep -Fxq %s; curl -fsS --max-time 10 http://127.0.0.1:%d/ >/dev/null; ss -lunH | awk '{print $5}' | grep -Eq '(^|:)%d$'; ss -ltnH | awk '{print $4}' | grep -Eq '(^|:)%d$'; (docker exec %s awg show || docker exec %s wg show) >/dev/null; printf 'HEALTH=ok\\n'", shellQuote(s.Container), shellQuote(s.Container), shellQuote(s.Image), s.WebPort, s.VPNPort, s.WebPort, shellQuote(s.Container), shellQuote(s.Container))
}

func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'" }
