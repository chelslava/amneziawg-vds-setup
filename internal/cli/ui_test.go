package cli

import (
	"bufio"
	"strings"
	"testing"
)

func TestInteractiveInstallBuildsSafeArguments(t *testing.T) {
	input := bufio.NewReader(strings.NewReader("vpn.example.com\nroot\n22\nC:\\Users\\me\\.ssh\\id_ed25519\n\nlegacy\n1234\n51821\nvpn.example.com\nyes\n\n"))
	args, err := interactiveCommand(input, &strings.Builder{}, "1")
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(args, " ")
	for _, want := range []string{"install", "--host vpn.example.com", "--identity-file C:\\Users\\me\\.ssh\\id_ed25519", "--engine legacy", "--vpn-port 1234", "--web-port 51821", "--domain vpn.example.com", "--tls"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("interactive install lacks %q: %s", want, joined)
		}
	}
	if strings.Contains(strings.ToLower(joined), "password") {
		t.Fatalf("interactive form must not create password arguments: %s", joined)
	}
}

func TestInteractiveExistingCommandUsesConnectionOnly(t *testing.T) {
	input := bufio.NewReader(strings.NewReader("198.51.100.10\nadmin\n2222\n\nknown_hosts\n"))
	args, err := interactiveCommand(input, &strings.Builder{}, "2")
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(args, " ")
	if joined != "status --host 198.51.100.10 --user admin --ssh-port 2222 --known-hosts known_hosts" {
		t.Fatalf("unexpected status arguments: %s", joined)
	}
}

func TestInteractiveMenuExit(t *testing.T) {
	var out strings.Builder
	if err := interactiveMenu(bufio.NewReader(strings.NewReader("0\n")), &out, &strings.Builder{}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Goodbye.") || !strings.Contains(out.String(), "control center") {
		t.Fatalf("menu output is incomplete: %s", out.String())
	}
}
