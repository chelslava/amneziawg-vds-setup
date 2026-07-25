package cli

import (
	"strings"
	"testing"

	"github.com/chelslava/amneziawg-vds-setup/v2/internal/config"
	"github.com/chelslava/amneziawg-vds-setup/v2/internal/preflight"
	"github.com/chelslava/amneziawg-vds-setup/v2/internal/state"
	tlsengine "github.com/chelslava/amneziawg-vds-setup/v2/internal/tls"
)

func TestParseInstallArguments(t *testing.T) {
	o, _, err := parse([]string{"install", "--host", "vpn.example", "--ssh-port", "2222", "--user", "admin", "--engine", "upstream", "--vpn-port", "443", "--web-port", "8443", "--domain", "vpn.example", "--tls"})
	if err != nil {
		t.Fatal(err)
	}
	if o.Host != "vpn.example" || o.SSHPort != 2222 || o.User != "admin" || o.Engine != config.Upstream || o.VPNPort != 443 || o.WebPort != 8443 || !o.TLS {
		t.Fatalf("unexpected options: %+v", o)
	}
}

func TestInstallUsesLongerDefaultSSHTimeout(t *testing.T) {
	install, _, err := parse([]string{"install", "--host", "vpn.example"})
	if err != nil {
		t.Fatal(err)
	}
	if install.TimeoutSecs != 900 {
		t.Fatalf("install timeout = %d, want 900", install.TimeoutSecs)
	}
	status, _, err := parse([]string{"status", "--host", "vpn.example"})
	if err != nil {
		t.Fatal(err)
	}
	if status.TimeoutSecs != 120 {
		t.Fatalf("status timeout = %d, want 120", status.TimeoutSecs)
	}
}

func TestPasswordArgumentsRejected(t *testing.T) {
	if !containsSecretFlag([]string{"install", "--host", "x", "--password=secret"}) {
		t.Fatal("password argument was not rejected")
	}
	if containsSecretFlag([]string{"install", "--host", "x", "--identity-file", "id_ed25519"}) {
		t.Fatal("identity flag was incorrectly rejected")
	}
}

func TestAutomaticMigrationRejected(t *testing.T) {
	s := state.State{Engine: config.Legacy}
	if err := ValidateEngineReuse(s, config.Upstream); err == nil || !strings.Contains(err.Error(), "automatic migration") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDomainRequiresTLS(t *testing.T) {
	o, _, err := parse([]string{"install", "--host", "vpn.example", "--domain", "vpn.example"})
	if err != nil {
		t.Fatal(err)
	}
	if err := o.ValidateInstall(); err == nil || !strings.Contains(err.Error(), "requires --tls") {
		t.Fatalf("expected domain/TLS validation error, got %v", err)
	}
}

func TestTLSCommandUsesExplicitMode(t *testing.T) {
	if got := tlsengine.Command(false, "vpn.example.com", 51821); !strings.Contains(got, "docker rm -f awg-vds-caddy") {
		t.Fatalf("disabled TLS must remove the proxy: %s", got)
	}
	got := tlsengine.Command(true, "vpn.example.com", 51821)
	for _, want := range []string{"vpn.example.com", "reverse_proxy 127.0.0.1:51821", tlsengine.Image, "--network host"} {
		if !strings.Contains(got, want) {
			t.Fatalf("TLS command lacks %q: %s", want, got)
		}
	}
}

func TestConfigurationDriftIsReported(t *testing.T) {
	s := state.State{VPNPort: 1234, WebPort: 51821, Domain: "vpn.example.com", TLSMode: "caddy", RestrictPanelIP: "198.51.100.7"}
	o := config.Options{VPNPort: 4321, WebPort: 51822, Domain: "other.example.com", TLS: false, RestrictIP: "198.51.100.8"}
	diff := configurationDrift(s, o)
	if len(diff) != 5 {
		t.Fatalf("expected five drift fields, got %v", diff)
	}
	if !strings.Contains(strings.Join(diff, " "), "vpn-port") || !strings.Contains(strings.Join(diff, " "), "restrict-panel-ip") {
		t.Fatalf("drift details are incomplete: %v", diff)
	}
}

func TestInstallValidationTLSMatrix(t *testing.T) {
	tests := []struct {
		name   string
		domain string
		tls    bool
		want   string
	}{
		{name: "host only"},
		{name: "domain with tls", domain: "vpn.example.com", tls: true},
		{name: "domain without tls", domain: "vpn.example.com", want: "requires --tls"},
		{name: "tls without domain", tls: true, want: "requires --domain"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o := config.Options{Host: "192.0.2.1", User: "root", SSHPort: 22, Engine: config.Legacy, VPNPort: 1234, WebPort: 51821, Domain: tt.domain, TLS: tt.tls}
			err := o.ValidateInstall()
			if tt.want == "" && err != nil {
				t.Fatalf("unexpected validation error: %v", err)
			}
			if tt.want != "" && (err == nil || !strings.Contains(err.Error(), tt.want)) {
				t.Fatalf("expected %q, got %v", tt.want, err)
			}
		})
	}
}

func TestUpstreamPreflightRefreshesMetadata(t *testing.T) {
	cmd := preflight.Command(config.Options{Engine: config.Upstream, WebPort: 51821, VPNPort: 1234})
	if !strings.Contains(cmd, "apt-get update -qq") || !strings.Contains(cmd, "AMNEZIAWG=repository-unavailable") {
		t.Fatalf("upstream preflight lacks deterministic repository diagnostics: %s", cmd)
	}
}

func TestPanelURLUsesPersistedHostAndPort(t *testing.T) {
	hostOnly := state.State{PanelHost: "198.51.100.7", WebPort: 51821, TLSMode: "disabled"}
	if got := panelURL(hostOnly); got != "http://198.51.100.7:51821" {
		t.Fatalf("unexpected host-only panel URL: %s", got)
	}
	tls := state.State{Domain: "vpn.example.com", PanelHost: "vpn.example.com", WebPort: 51821, TLSMode: "caddy"}
	if got := panelURL(tls); got != "https://vpn.example.com" {
		t.Fatalf("unexpected TLS panel URL: %s", got)
	}
}

func TestDependenciesIncludeHealthAndModuleDiagnostics(t *testing.T) {
	cmd := dependenciesCommand(true)
	for _, want := range []string{"apt-cache policy \"$package\"", "docker.io docker-ce-cli docker-cli", "APT::Get::AllowUnauthenticated=false install -y ca-certificates apache2-utils openssl curl", "APT::Get::AllowUnauthenticated=false full-upgrade -y", "Acquire::AllowInsecureRepositories=false", "docker-compose-plugin docker-compose-v2 docker-compose", "AMNEZIAWG=kernel-upgrade-failed", "apt-cache policy linux-headers-$(uname -r)", "Candidate: [^()]", "AMNEZIAWG=kernel-headers-unavailable-after-upgrade", "linux-headers-$(uname -r)", "amneziawg-dkms amneziawg-tools", "AMNEZIAWG=package-install-failed", "AMNEZIAWG=module-load-failed"} {
		if !strings.Contains(cmd, want) {
			t.Fatalf("dependency command lacks %q: %s", want, cmd)
		}
	}
}

func TestSupportedThirdPartyOS(t *testing.T) {
	for _, id := range []string{"ubuntu", "fedora", "rhel", "centos", "rocky", "almalinux", "ol"} {
		if !supportedThirdPartyOS("OS=" + id + " 9") {
			t.Fatalf("expected %s to support repository prompt", id)
		}
	}
	if supportedThirdPartyOS("OS=debian 12") {
		t.Fatal("Debian must not use the third-party repository prompt")
	}
}

func TestInstallHealthUsesRetryWindow(t *testing.T) {
	cmd := healthRetryCommand(state.State{Container: "awg-vds-legacy", Image: config.LegacyImage, VPNPort: 1234, WebPort: 51821})
	for _, want := range []string{"while test $attempt -le 6", "sleep 5", "docker inspect", "exit 1"} {
		if !strings.Contains(cmd, want) {
			t.Fatalf("health retry command lacks %q: %s", want, cmd)
		}
	}
}

func TestThirdPartyAmneziaRepositoryCommandUsesOfficialSignedPPA(t *testing.T) {
	command := thirdPartyAmneziaRepositoryCommand()
	for _, want := range []string{"add-apt-repository -y ppa:amnezia/ppa", "dnf copr enable -y amneziavpn/amneziawg", "AllowUnauthenticated=false", "AMNEZIAWG_REPOSITORY=official-launchpad-ppa", "AMNEZIAWG_REPOSITORY=official-copr"} {
		if !strings.Contains(command, want) {
			t.Fatalf("repository command missing %q: %s", want, command)
		}
	}
}
