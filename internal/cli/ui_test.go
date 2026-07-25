package cli

import (
	"bufio"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestInteractiveInstallBuildsSafeArguments(t *testing.T) {
	input := bufio.NewReader(strings.NewReader("vpn.example.com\nroot\n22\nkey\nC:\\Users\\me\\.ssh\\id_ed25519\n\nlegacy\n1234\n51821\nvpn.example.com\nyes\n\n"))
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
	input := bufio.NewReader(strings.NewReader("198.51.100.10\nadmin\n2222\nkey\n\nknown_hosts\n"))
	args, err := interactiveCommand(input, &strings.Builder{}, "2")
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(args, " ")
	if joined != "status --host 198.51.100.10 --user admin --ssh-port 2222 --known-hosts known_hosts" {
		t.Fatalf("unexpected status arguments: %s", joined)
	}
}

func TestInteractivePasswordAuthDoesNotCreatePasswordArgument(t *testing.T) {
	input := bufio.NewReader(strings.NewReader("198.51.100.10\nroot\n22\npassword\nknown_hosts\n"))
	var out strings.Builder
	args, err := interactiveCommand(input, &out, "2")
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(args, " ")
	if strings.Contains(strings.ToLower(joined), "password") || strings.Contains(joined, "identity-file") {
		t.Fatalf("password auth leaked into command arguments: %s", joined)
	}
	if !strings.Contains(out.String(), "requested interactively by OpenSSH") {
		t.Fatalf("password auth guidance missing: %s", out.String())
	}
}

func TestTUIViewContainsControlCenter(t *testing.T) {
	view := tuiModel{}.View().Content
	if !strings.Contains(view, "control center") || !strings.Contains(view, "Choose language") {
		t.Fatalf("TUI view is incomplete: %s", view)
	}
}

func TestTUILanguageSelectionPrecedesActionMenu(t *testing.T) {
	model := tuiModel{}
	updated, _ := model.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	model = updated.(tuiModel)
	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(tuiModel)
	if model.choice.language != langRU || model.step != 1 {
		t.Fatalf("language selection was not applied: %+v", model)
	}
	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	model = updated.(tuiModel)
	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(tuiModel)
	if model.choice.action != "status" {
		t.Fatalf("action selection was not applied: %+v", model)
	}
}
