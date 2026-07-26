package ssh

import (
	"bytes"
	"runtime"
	"strings"
	"testing"

	"github.com/chelslava/amneziawg-vds-setup/v2/internal/config"
)

func TestPreCommandTransportFailureDetection(t *testing.T) {
	for _, detail := range []string{
		"Connection timed out during banner exchange\nConnection to 203.0.113.10 port 22 timed out",
		"kex_exchange_identification: Connection closed by remote host",
	} {
		if !isPreCommandTransportFailure(detail) {
			t.Fatalf("expected retryable transport failure: %q", detail)
		}
	}
	for _, detail := range []string{
		"bash: docker: command not found",
		"E: Package linux-headers has no installation candidate",
	} {
		if isPreCommandTransportFailure(detail) {
			t.Fatalf("unexpected retryable remote failure: %q", detail)
		}
	}
}
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

func TestRemoteCommandsUseSystemPath(t *testing.T) {
	command := withSystemPath("command -v docker")
	if !strings.Contains(command, "/usr/bin") || !strings.HasPrefix(command, "export PATH=") || !strings.HasSuffix(command, "command -v docker") {
		t.Fatalf("remote command did not receive system PATH: %q", command)
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

func TestWindowsDoesNotEnableUnixControlSocket(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Win32-OpenSSH behavior only applies on Windows")
	}
	c := Client{Options: config.Options{Host: "vpn.example.com", User: "root", SSHPort: 22}}
	cleanup, err := c.EnableConnectionReuse()
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	if c.ControlPath != "" {
		t.Fatalf("Windows client must not configure ControlPath: %q", c.ControlPath)
	}
}

func TestWriteRedactedFiltersStderr(t *testing.T) {
	var out bytes.Buffer
	writeRedacted(&out, "PASSWORD_HASH=secret\nstatus=healthy\n")
	if strings.Contains(out.String(), "secret") || !strings.Contains(out.String(), "healthy") {
		t.Fatalf("stderr redaction failed: %q", out.String())
	}
}

func TestRedactedWriterFiltersStreamedSecrets(t *testing.T) {
	var out bytes.Buffer
	w := redactedWriter{w: &out}
	if _, err := w.Write([]byte("PASSWORD_HASH=secret\nstatus=healthy\n")); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "secret") || !strings.Contains(out.String(), "healthy") {
		t.Fatalf("streamed redaction failed: %q", out.String())
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
