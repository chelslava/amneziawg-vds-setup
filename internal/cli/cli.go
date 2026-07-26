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
	if len(args) == 0 {
		return interactiveTUI(os.Stdin, out, errOut)
	}
	if args[0] == "help" || args[0] == "--help" {
		usage(out)
		return nil
	}
	return runCommand(args, out, errOut)
}

func runCommand(args []string, out, errOut io.Writer) error {
	return runCommandWithPrompt(args, out, errOut, nil)
}

type thirdPartyRepoPrompt func() bool

func runCommandWithPrompt(args []string, out, errOut io.Writer, prompt thirdPartyRepoPrompt) error {
	return runCommandWithPromptContext(context.Background(), args, out, errOut, prompt, "")
}

func runCommandWithPromptContext(ctx context.Context, args []string, out, errOut io.Writer, prompt thirdPartyRepoPrompt, password string) error {
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
	if streamed, ok := out.(interface{ StreamedOutput() bool }); ok && streamed.StreamedOutput() {
		c.Stdout = out
	}
	if password != "" {
		c.SetPassword(password)
	}
	defer c.ForgetPassword()
	cleanupSSH, err := c.EnableConnectionReuse()
	if err != nil {
		return fmt.Errorf("prepare SSH connection reuse: %w", err)
	}
	defer cleanupSSH()
	switch o.Command {
	case "doctor":
		return doctor(ctx, &c, o, out)
	case "install":
		return installWithPrompt(ctx, &c, o, out, prompt)
	case "status":
		return existing(ctx, &c, o, out, "status")
	case "update":
		return existing(ctx, &c, o, out, "update")
	case "backup":
		return existing(ctx, &c, o, out, "backup")
	case "rotate-password":
		return existing(ctx, &c, o, out, "rotate-password")
	default:
		return fmt.Errorf("unknown command %q (use install, status, update, backup, rotate-password, or doctor)", o.Command)
	}
}

func parse(args []string) (config.Options, *flag.FlagSet, error) {
	defaultTimeout := 120
	if args[0] == "install" {
		defaultTimeout = 1800
	}
	o := config.Options{Command: args[0], SSHPort: 22, User: "root", Engine: config.Legacy, VPNPort: 1234, WebPort: 51821, TimeoutSecs: defaultTimeout}
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
	fs.IntVar(&o.TimeoutSecs, "timeout", defaultTimeout, "SSH command timeout in seconds")
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
		return errors.New("upstream preflight could not refresh package metadata; check repository connectivity, signatures, and supported distro repositories")
	}
	return nil
}

func install(ctx context.Context, c remoteRunner, o config.Options, out io.Writer) error {
	return installWithPrompt(ctx, c, o, out, nil)
}

func installWithPrompt(ctx context.Context, c remoteRunner, o config.Options, out io.Writer, prompt thirdPartyRepoPrompt) error {
	const totalSteps = 9
	installStep(out, 1, totalSteps, "Checking existing installation state")
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
		installStep(out, 2, totalSteps, "Running preflight checks")
		ssh.PrintOutput(out, pre)
		if err != nil {
			installFailed(out, "Running preflight checks", err)
			return err
		}
		installDone(out, "Running preflight checks")
		fmt.Fprintln(out, "Existing installation found; reconciling the selected engine without replacing configuration.")
		return reconcile(ctx, c, old, out)
	}
	installStep(out, 2, totalSteps, "Running preflight checks")
	pre, err := c.Run(ctx, preflight.Command(o))
	ssh.PrintOutput(out, pre)
	if err != nil {
		installFailed(out, "Running preflight checks", err)
		return err
	}
	installDone(out, "Running preflight checks")
	if strings.Contains(pre, "PORT_TCP_") && strings.Contains(pre, "=busy") {
		return errors.New("requested port is already occupied; inspect doctor output before installing")
	}
	if o.Engine == config.Upstream && (strings.Contains(pre, "AMNEZIAWG=unsupported") || strings.Contains(pre, "AMNEZIAWG=repository-unavailable")) {
		if prompt != nil && supportedThirdPartyOS(pre) && prompt() {
			if result, repoErr := c.Run(ctx, thirdPartyAmneziaRepositoryCommand()); repoErr != nil {
				ssh.PrintOutput(out, result)
				return fmt.Errorf("add official AmneziaWG repository: %w", repoErr)
			}
			retry, retryErr := c.Run(ctx, preflight.Command(o))
			ssh.PrintOutput(out, retry)
			if retryErr != nil {
				return retryErr
			}
			pre = retry
		}
		if strings.Contains(pre, "AMNEZIAWG=unsupported") || strings.Contains(pre, "AMNEZIAWG=repository-unavailable") {
			return errors.New("upstream requires the AmneziaWG kernel module; doctor found no supported installed or installable module")
		}
	}
	installStep(out, 3, totalSteps, "Preparing Docker and AmneziaWG dependencies")
	if result, err := c.Run(ctx, dependenciesCommand(o.Engine == config.Upstream)); err != nil {
		ssh.PrintOutput(out, result)
		installFailed(out, "Preparing Docker and AmneziaWG dependencies", err)
		return err
	}
	installDone(out, "Preparing Docker and AmneziaWG dependencies")
	s := newState(o)
	installStep(out, 4, totalSteps, "Preparing protected service configuration")
	if result, err := c.Run(ctx, envCommand(o, s)); err != nil {
		ssh.PrintOutput(out, result)
		installFailed(out, "Preparing protected service configuration", err)
		return err
	}
	installDone(out, "Preparing protected service configuration")
	e, _ := engine.Select(o.Engine)
	installStep(out, 5, totalSteps, "Starting the selected VPN engine")
	if result, err := c.Run(ctx, e.InstallCommand(s)); err != nil {
		ssh.PrintOutput(out, result)
		installFailed(out, "Starting the selected VPN engine", err)
		return err
	}
	installDone(out, "Starting the selected VPN engine")
	installStep(out, 6, totalSteps, "Configuring panel TLS")
	if result, err := c.Run(ctx, tlsengine.Command(o.TLS, o.Domain, o.WebPort)); err != nil {
		ssh.PrintOutput(out, result)
		installFailed(out, "Configuring panel TLS", err)
		return err
	}
	installDone(out, "Configuring panel TLS")
	installStep(out, 7, totalSteps, "Configuring firewall and port access")
	if result, err := c.Run(ctx, firewall.Command(o.VPNPort, o.WebPort, o.TLS, o.RestrictIP)); err != nil {
		ssh.PrintOutput(out, result)
		installFailed(out, "Configuring firewall and port access", err)
		return err
	}
	installDone(out, "Configuring firewall and port access")
	installStep(out, 8, totalSteps, "Checking containers, HTTP panel, and UDP listener")
	if result, err := c.Run(ctx, healthRetryCommand(s)); err != nil {
		ssh.PrintOutput(out, result)
		installFailed(out, "Checking containers, HTTP panel, and UDP listener", err)
		return fmt.Errorf("post-install health check failed: %w", err)
	}
	installDone(out, "Checking containers, HTTP panel, and UDP listener")
	s.UpdatedAt = time.Now().UTC()
	installStep(out, 9, totalSteps, "Saving installation state")
	if err := writeState(ctx, c, s); err != nil {
		installFailed(out, "Saving installation state", err)
		return err
	}
	installDone(out, "Saving installation state")
	printSummary(out, s)
	return nil
}

func supportedThirdPartyOS(preflightOutput string) bool {
	for _, id := range []string{"ubuntu", "fedora", "rhel", "centos", "rocky", "almalinux", "ol"} {
		if strings.Contains(preflightOutput, "OS="+id+" ") {
			return true
		}
	}
	return false
}

func readPanelPasswordTUI(ctx context.Context, args []string, password string, errOut io.Writer) (string, error) {
	o, fs, err := parse(args)
	if err != nil {
		return "", err
	}
	if fs != nil && fs.NArg() > 0 {
		return "", fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}
	if err := o.ValidateConnection(); err != nil {
		return "", err
	}
	c := ssh.Client{Options: o, Stderr: io.Discard}
	if errOut != nil {
		c.Stderr = errOut
	}
	if password != "" {
		c.SetPassword(password)
	}
	defer c.ForgetPassword()
	cleanup, err := c.EnableConnectionReuse()
	if err != nil {
		return "", fmt.Errorf("prepare SSH connection reuse: %w", err)
	}
	defer cleanup()
	result, err := c.Run(ctx, "umask 077; test -s /opt/awg-vds/panel-password; cat /opt/awg-vds/panel-password")
	if err != nil {
		return "", err
	}
	result = strings.TrimSpace(result)
	if result == "" || strings.ContainsAny(result, "\\r\\n") {
		return "", errors.New("panel password file is empty or malformed")
	}
	return result, nil
}

func healthRetryCommand(s state.State) string {
	check := health.Command(s)
	return fmt.Sprintf("set -eu; attempt=1; while test $attempt -le 6; do if %s; then stable=1; while test $stable -lt 3; do sleep 2; %s; stable=$((stable+1)); done; exit 0; fi; sleep 5; attempt=$((attempt+1)); done; exit 1", check, check)
}

func installStep(out io.Writer, current, total int, label string) {
	if out == nil {
		return
	}
	filled := current * 20 / total
	bar := strings.Repeat("█", filled) + strings.Repeat("░", 20-filled)
	fmt.Fprintf(out, "\n[%s] %d/%d  %s  (%s)\n", bar, current, total, label, time.Now().Format("15:04:05"))
}

func installDone(out io.Writer, label string) {
	if out != nil {
		fmt.Fprintf(out, "  ✓ %s completed (%s)\n", label, time.Now().Format("15:04:05"))
	}
}

func installFailed(out io.Writer, label string, err error) {
	if out != nil {
		fmt.Fprintf(out, "  ✗ %s failed: %s\n", label, ssh.Redact(err.Error()))
	}
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
	case "rotate-password":
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
		result, err = c.Run(ctx, rotatePasswordCommand(s))
		ssh.PrintOutput(out, result)
		if err != nil {
			return fmt.Errorf("credential rotation failed; previous credential was restored: %w", err)
		}
		if result, err = c.Run(ctx, health.Command(s)); err != nil {
			ssh.PrintOutput(out, result)
			rollbackResult, rollbackErr := c.Run(ctx, rollbackPasswordCommand(s))
			ssh.PrintOutput(out, rollbackResult)
			if rollbackErr != nil {
				return fmt.Errorf("credential health check failed: %v; credential rollback failed: %w", err, rollbackErr)
			}
			return fmt.Errorf("credential health check failed; previous credential restored: %w", err)
		}
		ssh.PrintOutput(out, result)
		if _, err := c.Run(ctx, cleanupPasswordRotationCommand()); err != nil {
			return fmt.Errorf("credential rotation succeeded but cleanup failed: %w", err)
		}
		fmt.Fprintln(out, "Panel credential rotated successfully.")
		fmt.Fprintln(out, "Retrieve it interactively from /opt/awg-vds/panel-password; it is never printed by awg-vds.")
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
	upstreamDebian := ""
	upstreamRPM := ""
	upstreamMarker := ""
	if upstream {
		upstreamDebian = "if ! apt-get -o APT::Get::AllowUnauthenticated=false full-upgrade -y; then printf 'AMNEZIAWG=kernel-upgrade-failed\\n' >&2; exit 1; fi; apt-get -o Acquire::AllowInsecureRepositories=false -o APT::Get::AllowUnauthenticated=false update; if ! apt-cache policy linux-headers-$(uname -r) 2>/dev/null | grep -q 'Candidate: [^()]'; then reboot_required=0; test -e /var/run/reboot-required && reboot_required=1 || true; printf 'AMNEZIAWG=kernel-headers-unavailable-after-upgrade kernel=%s reboot-required=%s\\n' \"$(uname -r)\" \"$reboot_required\" >&2; exit 1; fi; apt-get -o APT::Get::AllowUnauthenticated=false install -y linux-headers-$(uname -r) dkms build-essential;"
		upstreamRPM = "if ! dnf upgrade -y --refresh; then printf 'AMNEZIAWG=kernel-upgrade-failed\\n' >&2; exit 1; fi; dnf install -y kernel-devel-$(uname -r) kernel-headers dkms gcc make;"
		upstreamMarker = "if ! test -e /sys/module/amneziawg && ! command -v awg >/dev/null 2>&1; then case \"$ID\" in ubuntu|debian) apt-get -o Acquire::AllowInsecureRepositories=false -o APT::Get::AllowUnauthenticated=false update; if ! apt-get -o APT::Get::AllowUnauthenticated=false install -y amneziawg; then printf 'AMNEZIAWG=package-install-failed\\n' >&2; exit 1; fi ;; fedora|rhel|centos|rocky|almalinux|ol) if ! dnf install -y amneziawg-dkms amneziawg-tools; then printf 'AMNEZIAWG=package-install-failed\\n' >&2; exit 1; fi ;; *) printf 'AMNEZIAWG=unsupported\\n' >&2; exit 1 ;; esac; if module_error=$(modprobe amneziawg 2>&1); then printf 'AMNEZIAWG=module-loaded\\n'; else printf 'AMNEZIAWG=module-load-failed %%s\\n' \"$module_error\" >&2; exit 1; fi; fi; "
	}
	return "set -eu; . /etc/os-release; export DEBIAN_FRONTEND=noninteractive NEEDRESTART_MODE=a; case \"$ID\" in ubuntu|debian) apt-get -o Acquire::AllowInsecureRepositories=false -o APT::Get::AllowUnauthenticated=false update; " +
		upstreamDebian + " apt-get -o APT::Get::AllowUnauthenticated=false install -y ca-certificates apache2-utils openssl curl; if ! command -v docker >/dev/null 2>&1; then daemon_pkg=; cli_pkg=; if apt-cache policy docker.io 2>/dev/null | grep -q 'Candidate: [^()]'; then daemon_pkg=docker.io; elif apt-cache policy docker-ce 2>/dev/null | grep -q 'Candidate: [^()]'; then daemon_pkg=docker-ce; fi; for package in docker-cli docker-ce-cli; do if apt-cache policy \"$package\" 2>/dev/null | grep -q 'Candidate: [^()]'; then cli_pkg=\"$package\"; break; fi; done; test -n \"$daemon_pkg\"; docker_pkgs=\"$daemon_pkg\"; if test -n \"$cli_pkg\"; then docker_pkgs=\"$docker_pkgs $cli_pkg\"; fi; apt-get -o APT::Get::AllowUnauthenticated=false install -y --reinstall $docker_pkgs; fi; if ! docker compose version >/dev/null 2>&1 && ! command -v docker-compose >/dev/null 2>&1; then compose_pkg=; for package in docker-compose-plugin docker-compose-v2 docker-compose; do if apt-cache show \"$package\" >/dev/null 2>&1; then compose_pkg=\"$package\"; break; fi; done; test -n \"$compose_pkg\"; apt-get -o APT::Get::AllowUnauthenticated=false install -y \"$compose_pkg\"; fi; command -v htpasswd >/dev/null 2>&1 || apt-get -o APT::Get::AllowUnauthenticated=false install -y apache2-utils ;; " +
		"fedora|rhel|centos|rocky|almalinux|ol) " + upstreamRPM + " dnf install -y ca-certificates httpd-tools openssl curl docker; if ! docker compose version >/dev/null 2>&1 && ! command -v docker-compose >/dev/null 2>&1; then dnf install -y docker-compose-plugin || dnf install -y docker-compose; fi ;; *) printf 'ERROR=Unsupported OS: %s\\n' \"$ID\" >&2; exit 1 ;; esac; hash -r 2>/dev/null || true; if ! command -v docker >/dev/null 2>&1; then printf 'DOCKER=missing\\n' >&2; exit 1; fi; command -v openssl >/dev/null 2>&1 || exit 1; command -v curl >/dev/null 2>&1 || exit 1; systemctl enable --now docker; " + upstreamMarker + "printf 'DEPENDENCIES=ok\\n'"
}

func thirdPartyAmneziaRepositoryCommand() string {
	return "set -eu; . /etc/os-release; export DEBIAN_FRONTEND=noninteractive NEEDRESTART_MODE=a; case \"$ID\" in ubuntu) apt-get -o Acquire::AllowInsecureRepositories=false -o APT::Get::AllowUnauthenticated=false update; apt-get -o APT::Get::AllowUnauthenticated=false full-upgrade -y; apt-get -o APT::Get::AllowUnauthenticated=false install -y software-properties-common python3-launchpadlib gnupg2; add-apt-repository -y ppa:amnezia/ppa; apt-get -o Acquire::AllowInsecureRepositories=false -o APT::Get::AllowUnauthenticated=false update; printf 'AMNEZIAWG_REPOSITORY=official-launchpad-ppa\\n' ;; fedora|rhel|centos|rocky|almalinux|ol) dnf install -y epel-release || true; dnf install -y dnf-plugins-core; dnf copr enable -y amneziavpn/amneziawg; dnf makecache; printf 'AMNEZIAWG_REPOSITORY=official-copr\\n' ;; *) printf 'AMNEZIAWG_REPOSITORY=unsupported\\n' >&2; exit 1 ;; esac"
}

func rotatePasswordCommand(s state.State) string {
	envName := "upstream.env"
	if s.Engine == config.Legacy {
		envName = "legacy.env"
	}
	envPath := "/opt/awg-vds/" + envName
	return fmt.Sprintf("set -eu; dir=/opt/awg-vds; env=%s; oldenv=$dir/.panel-rotation.env; oldpass=$dir/.panel-rotation.password; cp \"$env\" \"$oldenv\"; cp \"$dir/panel-password\" \"$oldpass\"; rollback(){ cp \"$oldenv\" \"$env\"; cp \"$oldpass\" \"$dir/panel-password\"; chmod 600 \"$env\" \"$dir/panel-password\"; docker restart %s >/dev/null 2>&1 || true; }; trap rollback ERR; new=$(openssl rand -hex 24); hash=$(printf '%%s\\n' \"$new\" | htpasswd -niBC 12 '' | cut -d: -f2-); awk -v h=\"$hash\" 'BEGIN{done=0} /^PASSWORD_HASH=/{print \"PASSWORD_HASH=\" h; done=1; next} {print} END{if(!done) exit 1}' \"$env\" >\"$env.tmp\"; printf '%%s\\n' \"$new\" >\"$dir/panel-password.tmp\"; chmod 600 \"$env.tmp\" \"$dir/panel-password.tmp\"; mv \"$env.tmp\" \"$env\"; mv \"$dir/panel-password.tmp\" \"$dir/panel-password\"; docker restart %s >/dev/null; trap - ERR; printf 'ROTATION=prepared\\n'", shellQuote(envPath), shellQuote(s.Container), shellQuote(s.Container))
}

func rollbackPasswordCommand(s state.State) string {
	envName := "upstream.env"
	if s.Engine == config.Legacy {
		envName = "legacy.env"
	}
	return fmt.Sprintf("set -eu; cp /opt/awg-vds/.panel-rotation.env /opt/awg-vds/%s; cp /opt/awg-vds/.panel-rotation.password /opt/awg-vds/panel-password; chmod 600 /opt/awg-vds/%s /opt/awg-vds/panel-password; docker restart %s >/dev/null; printf 'ROTATION=rolled-back\\n'", envName, envName, shellQuote(s.Container))
}

func cleanupPasswordRotationCommand() string {
	return "set -eu; rm -f /opt/awg-vds/.panel-rotation.env /opt/awg-vds/.panel-rotation.password; printf 'ROTATION=cleaned\\n'"
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
		external := externalPanelStatus(s)
		fmt.Fprintln(out, "External panel:", external)
		if strings.HasPrefix(external, "unreachable") {
			fmt.Fprintln(out, "WARNING: external panel check failed; verify UFW/firewalld, provider security-group rules, and that the panel is not bound to localhost.")
		}
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
	fmt.Fprintln(out, "awg-vds v2.2.1 — cross-platform AmneziaWG VDS installer")
	fmt.Fprintln(out, "Commands: install, status, update, backup, rotate-password, doctor")
	fmt.Fprintln(out, "Common flags: --host HOST --ssh-port 22 --user root --identity-file PATH --known-hosts PATH")
	fmt.Fprintln(out, "Install flags: --engine legacy|upstream --vpn-port 1234 --web-port 51821 --domain NAME --tls --restrict-panel-ip IP")
}
