package ssh

import (
	"bytes"
	"strings"
	"testing"

	"github.com/chelslava/amneziawg-vds-setup/v2/internal/config"
)

func TestRedactSecrets(t *testing.T) {
	input := "PASSWORD_HASH=$2y$secret\nPrivateKey=abc\nstatus=healthy\n"
	output := Redact(input)
	for _, secret := range []string{"$2y$secret", "abc"} {
		if contains(output, secret) {
			t.Fatalf("secret leaked: %q in %q", secret, output)
		}
	}
	if !contains(output, "healthy") {
		t.Fatal("safe status was removed")
	}
}

func TestArgsFailClosedAndAllowExplicitKnownHosts(t *testing.T) {
	c := Client{Options: config.Options{Host: "vpn.example.com", User: "root", SSHPort: 2222, KnownHosts: "/tmp/known_hosts"}}
	args := strings.Join(c.args(), " ")
	for _, want := range []string{"StrictHostKeyChecking=yes", "UserKnownHostsFile=/tmp/known_hosts"} {
		if !strings.Contains(args, want) {
			t.Fatalf("SSH args lack %q: %s", want, args)
		}
	}
	if strings.Contains(args, "accept-new") {
		t.Fatalf("SSH args still trust unknown first-connect keys: %s", args)
	}
}

func TestArgsEnableInteractivePasswordFallbackWithoutIdentity(t *testing.T) {
	c := Client{Options: config.Options{Host: "vpn.example.com", User: "root", SSHPort: 22}}
	args := strings.Join(c.args(), " ")
	for _, want := range []string{"PreferredAuthentications=publickey,password,keyboard-interactive", "PasswordAuthentication=yes", "KbdInteractiveAuthentication=yes"} {
		if !strings.Contains(args, want) {
			t.Fatalf("SSH args lack interactive fallback %q: %s", want, args)
		}
	}
}

func TestArgsReuseOneAuthenticatedSSHConnection(t *testing.T) {
	c := Client{Options: config.Options{Host: "vpn.example.com", User: "root", SSHPort: 22}, ControlPath: "C:\\Temp\\awg-vds-control"}
	args := strings.Join(c.args(), " ")
	for _, want := range []string{"ControlMaster=auto", "ControlPersist=120s", "ControlPath=C:\\Temp\\awg-vds-control"} {
		if !strings.Contains(args, want) {
			t.Fatalf("SSH args lack connection reuse option %q: %s", want, args)
		}
	}
}

func TestWriteRedactedFiltersStderr(t *testing.T) {
	var out bytes.Buffer
	writeRedacted(&out, "PASSWORD_HASH=secret\nstatus=healthy\n")
	if strings.Contains(out.String(), "secret") || !strings.Contains(out.String(), "healthy") {
		t.Fatalf("stderr redaction failed: %q", out.String())
	}
}

func contains(s, part string) bool {
	for i := 0; i+len(part) <= len(s); i++ {
		if s[i:i+len(part)] == part {
			return true
		}
	}
	return false
}
