package state

import (
	"strings"
	"testing"
	"time"

	"github.com/chelslava/amneziawg-vds-setup/v2/internal/config"
)

func TestEncodeDecodeStateContainsNoSecrets(t *testing.T) {
	s := State{Version: 1, Engine: config.Legacy, Image: "ghcr.io/yokitoki/awg-easy@sha256:bfb9070d88379dc31ce55ef5588915964a2c3abd657249c696dd375202df3f6f", Container: "awg-vds-legacy", VPNPort: 1234, WebPort: 51821, TLSMode: "disabled", ConfigPath: "/opt/awg-vds/wireguard", BackupPath: "/opt/awg-vds/backups", InstalledAt: time.Unix(1, 0).UTC(), UpdatedAt: time.Unix(1, 0).UTC()}
	b, err := Encode(s)
	if err != nil {
		t.Fatal(err)
	}
	text := string(b)
	for _, secret := range []string{"PASSWORD_HASH", "PrivateKey", "password", "client.conf"} {
		if strings.Contains(text, secret) {
			t.Fatalf("state contains secret marker %q: %s", secret, text)
		}
	}
	decoded, err := Decode(strings.NewReader(text))
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Engine != s.Engine || decoded.ConfigPath != s.ConfigPath {
		t.Fatalf("round-trip mismatch: %+v", decoded)
	}
}

func TestDecodeRejectsUnknownFields(t *testing.T) {
	_, err := Decode(strings.NewReader(`{"schema_version":1,"engine":"legacy","image":"x","container":"x","vpn_port":1,"web_port":2,"tls_mode":"disabled","config_path":"/x","backup_path":"/b","installed_at":"1970-01-01T00:00:01Z","updated_at":"1970-01-01T00:00:01Z","password":"bad"}`))
	if err == nil {
		t.Fatal("unknown secret field was accepted")
	}
}

func TestBackupMetadataMustBeComplete(t *testing.T) {
	s := State{Version: 1, Engine: config.Legacy, Image: config.LegacyImage, Container: "awg-vds-legacy", VPNPort: 1234, WebPort: 51821, TLSMode: "disabled", ConfigPath: "/opt/awg-vds/wireguard", BackupPath: "/opt/awg-vds/backups", LastBackupPath: "/opt/awg-vds/backups/a.tar.gz"}
	if _, err := Encode(s); err == nil {
		t.Fatal("incomplete backup metadata was accepted")
	}
	s.LastBackupSHA256 = "abc"
	if _, err := Encode(s); err != nil {
		t.Fatalf("complete backup metadata rejected: %v", err)
	}
}

func TestDomainAndTLSModeMustAgree(t *testing.T) {
	base := State{Version: 1, Engine: config.Legacy, Image: config.LegacyImage, Container: "awg-vds-legacy", VPNPort: 1234, WebPort: 51821, TLSMode: "disabled", ConfigPath: "/opt/awg-vds/wireguard", BackupPath: "/opt/awg-vds/backups"}
	base.Domain = "vpn.example.com"
	if _, err := Encode(base); err == nil {
		t.Fatal("domain without TLS mode was accepted")
	}
	base.TLSMode = "caddy"
	if _, err := Encode(base); err != nil {
		t.Fatalf("consistent domain/TLS state rejected: %v", err)
	}
}

func TestStateRejectsTamperedLifecycleFields(t *testing.T) {
	base := State{Version: 1, Engine: config.Legacy, Image: config.LegacyImage, Container: "awg-vds-legacy", VPNPort: 1234, WebPort: 51821, TLSMode: "disabled", ConfigPath: "/opt/awg-vds/wireguard", BackupPath: "/opt/awg-vds/backups"}
	tests := []struct {
		name string
		edit func(*State)
	}{
		{name: "path traversal", edit: func(s *State) { s.ConfigPath = "/opt/awg-vds/../etc" }},
		{name: "wrong container", edit: func(s *State) { s.Container = "attacker" }},
		{name: "wrong image", edit: func(s *State) { s.Image = "ghcr.io/attacker/image@sha256:" + strings.Repeat("a", 64) }},
		{name: "bad restriction", edit: func(s *State) { s.RestrictPanelIP = "not-an-ip" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := base
			tt.edit(&s)
			if _, err := Encode(s); err == nil {
				t.Fatal("tampered state was accepted")
			}
		})
	}
}
