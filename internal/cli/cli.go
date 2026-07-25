package cli

import (
	"context"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/chelslava/amneziawg-vds-setup/v2/internal/backup"
	"github.com/chelslava/amneziawg-vds-setup/v2/internal/config"
	"github.com/chelslava/amneziawg-vds-setup/v2/internal/engine"
	"github.com/chelslava/amneziawg-vds-setup/v2/internal/firewall"
	"github.com/chelslava/amneziawg-vds-setup/v2/internal/health"
	"github.com/chelslava/amneziawg-vds-setup/v2/internal/preflight"
	"github.com/chelslava/amneziawg-vds-setup/v2/internal/ssh"
	"github.com/chelslava/amneziawg-vds-setup/v2/internal/state"
	tlsengine "github.com/chelslava/amneziawg-vds-setup/v2/internal/tls"
)

type remoteRunner interface {
	Run(context.Context, string) (string, error)
}

func Run(args []string, out, errOut io.Writer) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" {
		usage(out)
		return nil
	}
	if containsSecretFlag(args) {
		return errors.New("password flags are forbidden; SSH password authentication is interactive only")
	}
	o, fs, err := parse(args)
	if err != nil {
		return err
	}
	if fs != nil && fs.NArg() > 0 {
		return fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}
	if o.Command == "install" {
		if err := o.ValidateInstall(); err != nil {
			return err
		}
	} else if err := o.ValidateConnection(); err != nil {
		return err
	}
	c := ssh.Client{Options: o, Stdin: os.Stdin, Stderr: errOut}
	ctx := context.Background()
	switch o.Command {
	case "doctor":
		return doctor(ctx, c, o, out)
	case "install":
		return install(ctx, c, o, out)
	case "status":
		return existing(ctx, c, o, out, "status")
	case "update":
		return existing(ctx, c, o, out, "update")
	case "backup":
		return existing(ctx, c, o, out, "backup")
	default:
		return fmt.Errorf("unknown command %q (use install, status, update, backup, or doctor)", o.Command)
	}
}

func parse(args []string) (config.Options, *flag.FlagSet, error) {
	o := config.Options{Command: args[0], SSHPort: 22, User: "root", Engine: config.Legacy, VPNPort: 1234, WebPort: 51821, TimeoutSecs: 120}
	fs := flag.NewFlagSet(args[0], flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&o.Host, "host", "", "VDS address")
	fs.IntVar(&o.SSHPort, "ssh-port", 22, "SSH port")
	fs.StringVar(&o.User, "user", "root", "SSH user")
	fs.StringVar(&o.Identity, "identity-file", "", "SSH private key path")
	fs.StringVar(&o.KnownHosts, "known-hosts", "", "SSH known_hosts file")
	fs.Var(engineValue{&o.Engine}, "engine", "legacy or upstream")
	fs.IntVar(&o.VPNPort, "vpn-port", 1234, "VPN UDP port")
	fs.IntVar(&o.WebPort, "web-port", 51821, "web TCP port")
	fs.StringVar(&o.Domain, "domain", "", "public DNS name")
	fs.BoolVar(&o.TLS, "tls", false, "enable pinned Caddy TLS proxy")
	fs.StringVar(&o.RestrictIP, "restrict-panel-ip", "", "allow direct panel access only from this IP when TLS is enabled")
	fs.BoolVar(&o.Force, "force", false, "reserved for explicit destructive operations")
	fs.IntVar(&o.TimeoutSecs, "timeout", 120, "SSH command timeout in seconds")
	if err := fs.Parse(args[1:]); err != nil {
		return o, fs, err
	}
	return o, fs, nil
}

type engineValue struct{ target *config.Engine }

func (v engineValue) String() string { return string(*v.target) }
func (v engineValue) Set(s string) error {
	e := config.Engine(s)
	if e != config.Legacy && e != config.Upstream {
		return fmt.Errorf("engine must be legacy or upstream")
	}
	*v.target = e
	return nil
}
func (v engineValue) Get() any { return *v.target }

func containsSecretFlag(args []string) bool {
	for _, a := range args {
		if strings.HasPrefix(a, "--password") || strings.HasPrefix(a, "--ssh-password") || strings.HasPrefix(a, "--private-key=") {
			return true
		}
	}
	return false
}

func doctor(ctx context.Context, c remoteRunner, o config.Options, out io.Writer) error {
	result, err := c.Run(ctx, preflight.Command(o))
	ssh.PrintOutput(out, result)
	if err != nil {
		return err
	}
	if strings.Contains(result, "AMNEZIAWG=unsupported") {
		return errors.New("upstream engine is unsupported: AmneziaWG module is neither installed nor available from configured repositories")
	}
	if strings.Contains(result, "AMNEZIAWG=repository-unavailable") {
		return errors.New("upstream preflight could not refresh APT metadata; check repository connectivity and signatures")
	}
	return nil
}

func install(ctx context.Context, c remoteRunner, o config.Options, out io.Writer) error {
	old, found, err := readState(ctx, c)
	if err != nil {
		return err
	}
	if found {
		if old.Engine != o.Engine {
			return fmt.Errorf("refusing automatic migration from %s to %s; install the other engine as a separate new installation", old.Engine, o.Engine)
		}
		if diff := configurationDrift(old, o); len(diff) > 0 {
			return fmt.Errorf("existing installation configuration differs: %s; rerun with the existing settings or use a future reconfigure flow", strings.Join(diff, ", "))
		}
		pre, err := c.Run(ctx, preflight.Command(o))
		ssh.PrintOutput(out, pre)
		if err != nil {
			return err
		}
		fmt.Fprintln(out, "Existing installation found; reconciling the selected engine without replacing configuration.")
		return reconcile(ctx, c, old, out)
	}
	pre, err := c.Run(ctx, preflight.Command(o))
	ssh.PrintOutput(out, pre)
	if err != nil {
		return err
	}
	if strings.Contains(pre, "PORT_TCP_") && strings.Contains(pre, "=busy") {
		return errors.New("requested port is already occupied; inspect doctor output before installing")
	}
	if o.Engine == config.Upstream && (strings.Contains(pre, "AMNEZIAWG=unsupported") || strings.Contains(pre, "AMNEZIAWG=repository-unavailable")) {
		return errors.New("upstream requires the AmneziaWG kernel module; doctor found no supported installed or installable module")
	}
	if result, err := c.Run(ctx, dependenciesCommand(o.Engine == config.Upstream)); err != nil {
		ssh.PrintOutput(out, result)
		return err
	}
	s := newState(o)
	if result, err := c.Run(ctx, envCommand(o, s)); err != nil {
		ssh.PrintOutput(out, result)
		return err
	}
	e, _ := engine.Select(o.Engine)
	if result, err := c.Run(ctx, e.InstallCommand(s)); err != nil {
		ssh.PrintOutput(out, result)
		return err
	}
	if result, err := c.Run(ctx, tlsengine.Command(o.TLS, o.Domain, o.WebPort)); err != nil {
		ssh.PrintOutput(out, result)
		return err
	}
	if result, err := c.Run(ctx, firewall.Command(o.VPNPort, o.WebPort, o.TLS, o.RestrictIP)); err != nil {
		ssh.PrintOutput(out, result)
		return err
	}
	if result, err := c.Run(ctx, health.Command(s)); err != nil {
		ssh.PrintOutput(out, result)
		return fmt.Errorf("post-install health check failed: %w", err)
	}
	s.UpdatedAt = time.Now().UTC()
	if err := writeState(ctx, c, s); err != nil {
		return err
	}
	printSummary(out, s)
	return nil
}

func existing(ctx context.Context, c remoteRunner, o config.Options, out io.Writer, action string) error {
	s, found, err := readState(ctx, c)
	if err != nil {
		return err
	}
	if !found {
		return errors.New("no v2 installation state found at /opt/awg-vds/install-state.json")
	}
	switch action {
	case "status":
		result, err := c.Run(ctx, health.Command(s))
		ssh.PrintOutput(out, result)
		if err != nil {
			return err
		}
		printSummary(out, s)
		return nil
	case "backup":
		result, err := c.Run(ctx, backup.Command(s))
		ssh.PrintOutput(out, result)
		if err != nil {
			return err
		}
		path, checksum, err := backup.ParseResult(result)
		if err != nil {
			return err
		}
		s.LastBackupPath, s.LastBackupSHA256 = path, checksum
		if err := writeState(ctx, c, s); err != nil {
			return err
		}
		fmt.Fprintln(out, "Backup created in", path)
		return nil
	case "update":
		result, err := c.Run(ctx, backup.Command(s))
		if err != nil {
			ssh.PrintOutput(out, result)
			return err
		}
		ssh.PrintOutput(out, result)
		backupPath, backupSHA, err := backup.ParseResult(result)
		if err != nil {
			return err
		}
		e, _ := engine.Select(s.Engine)
		candidate := s
		candidate.Image = e.Image()
		candidate.LastBackupPath, candidate.LastBackupSHA256 = backupPath, backupSHA
		result, err = c.Run(ctx, e.UpdateCommand(candidate))
		if err != nil {
			ssh.PrintOutput(out, result)
			return fmt.Errorf("update failed: %w; rollback: %v", err, rollback(ctx, c, s, backupPath, out))
		}
		ssh.PrintOutput(out, result)
		result, err = c.Run(ctx, tlsengine.Command(s.TLSMode == "caddy", s.Domain, s.WebPort))
		if err != nil {
			ssh.PrintOutput(out, result)
			return fmt.Errorf("TLS reconciliation failed: %w; rollback: %v", err, rollback(ctx, c, s, backupPath, out))
		}
		candidate.UpdatedAt = time.Now().UTC()
		result, err = c.Run(ctx, health.Command(candidate))
		if err != nil {
			ssh.PrintOutput(out, result)
			return fmt.Errorf("post-update health check failed: %w; rollback: %v", err, rollback(ctx, c, s, backupPath, out))
		}
		ssh.PrintOutput(out, result)
		if err := writeState(ctx, c, candidate); err != nil {
			return err
		}
		s = candidate
		fmt.Fprintln(out, "Update completed; the existing configuration was preserved.")
		printSummary(out, s)
		return nil
	}
	return fmt.Errorf("unknown action %s", action)
}

func reconcile(ctx context.Context, c remoteRunner, s state.State, out io.Writer) error {
	e, _ := engine.Select(s.Engine)
	candidate := s
	candidate.Image = e.Image()
	result, err := c.Run(ctx, e.UpdateCommand(candidate))
	if err != nil {
		ssh.PrintOutput(out, result)
		return err
	}
	ssh.PrintOutput(out, result)
	result, err = c.Run(ctx, health.Command(candidate))
	if err != nil {
		ssh.PrintOutput(out, result)
		return err
	}
	ssh.PrintOutput(out, result)
	if err := writeState(ctx, c, candidate); err != nil {
		return err
	}
	printSummary(out, candidate)
	return nil
}

func rollback(ctx context.Context, c remoteRunner, s state.State, archive string, out io.Writer) error {
	result, err := c.Run(ctx, backup.RestoreCommand(archive))
	ssh.PrintOutput(out, result)
	if err != nil {
		return err
	}
	e, err := engine.Select(s.Engine)
	if err != nil {
		return err
	}
	result, err = c.Run(ctx, e.UpdateCommand(s))
	ssh.PrintOutput(out, result)
	if err != nil {
		return err
	}
	result, err = c.Run(ctx, tlsengine.Command(s.TLSMode == "caddy", s.Domain, s.WebPort))
	ssh.PrintOutput(out, result)
	if err != nil {
		return err
	}
	result, err = c.Run(ctx, health.Command(s))
	ssh.PrintOutput(out, result)
	return err
}

func newState(o config.Options) state.State {
	now := time.Now().UTC()
	image := ""
	container := ""
	if e, _ := engine.Select(o.Engine); e != nil {
		image, container = e.Image(), e.Container()
	}
	tlsMode := "disabled"
	if o.TLS {
		tlsMode = "caddy"
	}
	return state.State{Version: 1, Engine: o.Engine, Image: image, Container: container, VPNPort: o.VPNPort, WebPort: o.WebPort, Domain: o.Domain, PanelHost: hostForPanel(o), RestrictPanelIP: o.RestrictIP, TLSMode: tlsMode, ConfigPath: "/opt/awg-vds/wireguard", BackupPath: "/opt/awg-vds/backups", InstalledAt: now, UpdatedAt: now}
}

func configurationDrift(s state.State, o config.Options) []string {
	var diff []string
	if s.VPNPort != o.VPNPort {
		diff = append(diff, fmt.Sprintf("vpn-port state=%d requested=%d", s.VPNPort, o.VPNPort))
	}
	if s.WebPort != o.WebPort {
		diff = append(diff, fmt.Sprintf("web-port state=%d requested=%d", s.WebPort, o.WebPort))
	}
	if s.Domain != o.Domain {
		diff = append(diff, fmt.Sprintf("domain state=%q requested=%q", s.Domain, o.Domain))
	}
	wantTLS := "disabled"
	if o.TLS {
		wantTLS = "caddy"
	}
	if s.TLSMode != wantTLS {
		diff = append(diff, fmt.Sprintf("tls state=%q requested=%q", s.TLSMode, wantTLS))
	}
	if s.RestrictPanelIP != o.RestrictIP && (s.RestrictPanelIP != "" || o.RestrictIP != "") {
		diff = append(diff, fmt.Sprintf("restrict-panel-ip state=%q requested=%q", s.RestrictPanelIP, o.RestrictIP))
	}
	return diff
}

func dependenciesCommand(upstream bool) string {
	cmd := "set -eu; export DEBIAN_FRONTEND=noninteractive NEEDRESTART_MODE=a; command -v docker >/dev/null 2>&1 || (apt-get update; apt-get install -y ca-certificates apache2-utils openssl curl docker.io docker-compose-plugin); command -v htpasswd >/dev/null 2>&1 || apt-get install -y apache2-utils; command -v openssl >/dev/null 2>&1 || apt-get install -y openssl; command -v curl >/dev/null 2>&1 || (apt-get update; apt-get install -y curl); systemctl enable --now docker; "
	if upstream {
		cmd += "if ! test -e /sys/module/amneziawg && ! command -v awg >/dev/null 2>&1; then apt-get update; if ! apt-get install -y amneziawg; then printf 'AMNEZIAWG=package-install-failed\\n' >&2; exit 1; fi; if module_error=$(modprobe amneziawg 2>&1); then printf 'AMNEZIAWG=module-loaded\\n'; else printf 'AMNEZIAWG=module-load-failed %s\\n' \"$module_error\" >&2; exit 1; fi; fi; "
	}
	return cmd + "printf 'DEPENDENCIES=ok\\n'"
}
func envCommand(o config.Options, s state.State) string {
	name := "upstream.env"
	if s.Engine == config.Legacy {
		name = "legacy.env"
	}
	return fmt.Sprintf("set -eu; install -d -m 700 /opt/awg-vds; umask 077; if test ! -s /opt/awg-vds/panel-password; then openssl rand -hex 24 > /opt/awg-vds/panel-password; fi; panel_hash=$(printf '%%s\\n' \"$(cat /opt/awg-vds/panel-password)\" | htpasswd -niBC 12 '' | cut -d: -f2-); tmp=$(mktemp /opt/awg-vds/env.XXXXXX); printf 'WG_HOST=%%s\\nPORT=%%d\\nWG_PORT=%%d\\nWG_PERSISTENT_KEEPALIVE=25\\nWG_DEFAULT_DNS=1.1.1.1,1.0.0.1\\nPASSWORD_HASH=%%s\\n' %s %d %d \"$panel_hash\" >\"$tmp\"; chmod 600 \"$tmp\"; mv \"$tmp\" /opt/awg-vds/%s; printf 'CONFIG=preserved\\n'", shellQuote(hostForPanel(o)), o.WebPort, o.VPNPort, name)
}
func hostForPanel(o config.Options) string {
	if o.Domain != "" {
		return o.Domain
	}
	return o.Host
}
func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'" }
func writeState(ctx context.Context, c remoteRunner, s state.State) error {
	b, err := state.Encode(s)
	if err != nil {
		return err
	}
	encoded := base64.StdEncoding.EncodeToString(b)
	_, err = c.Run(ctx, fmt.Sprintf("set -eu; install -d -m 700 /opt/awg-vds; printf '%s' | base64 -d > /opt/awg-vds/install-state.json.tmp; chmod 644 /opt/awg-vds/install-state.json.tmp; mv /opt/awg-vds/install-state.json.tmp %s", encoded, shellQuote(state.Path)))
	return err
}
func readState(ctx context.Context, c remoteRunner) (state.State, bool, error) {
	result, err := c.Run(ctx, "if test -f /opt/awg-vds/install-state.json; then base64 -w0 /opt/awg-vds/install-state.json; fi")
	if err != nil {
		return state.State{}, false, err
	}
	if strings.TrimSpace(result) == "" {
		return state.State{}, false, nil
	}
	b, err := base64.StdEncoding.DecodeString(strings.TrimSpace(result))
	if err != nil {
		return state.State{}, false, fmt.Errorf("decode remote state: %w", err)
	}
	s, err := state.Decode(strings.NewReader(string(b)))
	return s, true, err
}
func errOut(out io.Writer) io.Writer { return out }
func printSummary(out io.Writer, s state.State) {
	fmt.Fprintf(out, "Panel: %s\nVPN: UDP %d\nEngine: %s\nBackups: %s\nState: %s\n", panelURL(s), s.VPNPort, s.Engine, s.BackupPath, state.Path)
	if s.PanelHost != "" {
		fmt.Fprintln(out, "External panel:", externalPanelStatus(s))
	} else {
		fmt.Fprintln(out, "External panel: not checked (state has no panel host metadata)")
	}
	if s.TLSMode == "disabled" {
		fmt.Fprintln(out, "WARNING: web panel is plain HTTP; configure TLS or restrict panel access by IP.")
	}
}

func panelURL(s state.State) string {
	host := s.Domain
	if host == "" {
		host = s.PanelHost
	}
	if host == "" {
		return fmt.Sprintf("http://<VDS-address>:%d", s.WebPort)
	}
	if s.TLSMode == "caddy" {
		return "https://" + host
	}
	return "http://" + net.JoinHostPort(host, strconv.Itoa(s.WebPort))
}

func externalPanelStatus(s state.State) string {
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(panelURL(s))
	if err != nil {
		return "unreachable (" + err.Error() + ")"
	}
	defer resp.Body.Close()
	return fmt.Sprintf("reachable (HTTP %d)", resp.StatusCode)
}
func usage(out io.Writer) {
	fmt.Fprintln(out, "awg-vds v2.0.0 — cross-platform AmneziaWG VDS installer")
	fmt.Fprintln(out, "Commands: install, status, update, backup, doctor")
	fmt.Fprintln(out, "Common flags: --host HOST --ssh-port 22 --user root --identity-file PATH --known-hosts PATH")
	fmt.Fprintln(out, "Install flags: --engine legacy|upstream --vpn-port 1234 --web-port 51821 --domain NAME --tls --restrict-panel-ip IP")
}
