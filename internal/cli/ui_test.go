package cli

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
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
	if !strings.Contains(out.String(), "request the SSH password once without echo") {
		t.Fatalf("password auth guidance missing: %s", out.String())
	}
}

func TestTUIViewContainsControlCenter(t *testing.T) {
	view := tuiModel{}.View().Content
	if !strings.Contains(view, "control center") || !strings.Contains(view, "Choose language") {
		t.Fatalf("TUI view is incomplete: %s", view)
	}
}

func TestErrorNoticeExplainsMissingUpstreamModule(t *testing.T) {
	_, notice := errorNotice(langRU, "install", errors.New("upstream requires the AmneziaWG kernel module; doctor found no supported installed or installable module"))
	for _, want := range []string{"модуль ядра AmneziaWG", "Запустите doctor", "engine legacy", "автоматической миграции"} {
		if !strings.Contains(notice, want) {
			t.Fatalf("missing recommendation %q in %s", want, notice)
		}
	}
}

func TestErrorNoticeExplainsModuleLoadFailure(t *testing.T) {
	_, notice := errorNotice(langRU, "install", errors.New("AMNEZIAWG=module-load-failed modprobe: FATAL: Module amneziawg not found"))
	for _, want := range []string{"не загрузился", "linux-headers", "dkms", "engine legacy"} {
		if !strings.Contains(notice, want) {
			t.Fatalf("missing module-load recommendation %q in %s", want, notice)
		}
	}
}

func TestErrorNoticeExplainsMissingKernelHeaders(t *testing.T) {
	_, notice := errorNotice(langRU, "install", errors.New("SSH command failed: exit status 100: E: Package 'linux-headers-6.8.0-35-generic' has no installation candidate"))
	for _, want := range []string{"linux-headers", "apt full-upgrade", "перезагрузка", "engine legacy"} {
		if !strings.Contains(notice, want) {
			t.Fatalf("missing kernel-header recommendation %q in %s", want, notice)
		}
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

func TestTUILanguageOnlyScreenReturnsAfterSelection(t *testing.T) {
	model := tuiModel{languageOnly: true}
	updated, command := model.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	model = updated.(tuiModel)
	updated, command = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(tuiModel)
	if command == nil || model.choice.language != langRU || model.step != 0 {
		t.Fatalf("language screen did not finish cleanly: model=%+v command=%v", model, command)
	}
}

func TestUIPreferencesRoundTripContainsOnlyLanguage(t *testing.T) {
	b, err := json.Marshal(uiPreferences{Language: langRU})
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `{"language":"ru"}` {
		t.Fatalf("unexpected preference payload: %s", b)
	}
	var got uiPreferences
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got.Language != langRU {
		t.Fatalf("language was not persisted in preferences model: %+v", got)
	}
	if err := saveLanguagePreference(uiLanguage("xx")); err == nil {
		t.Fatal("unsupported language should be rejected")
	}
}

func TestInstallProgressRendersStageAndCompletionBar(t *testing.T) {
	var out strings.Builder
	installStep(&out, 9, 9, "Saving installation state")
	if !strings.Contains(out.String(), "[████████████████████] 9/9") || !strings.Contains(out.String(), "Saving installation state") {
		t.Fatalf("progress output is incomplete: %q", out.String())
	}
}

func TestErrorNoticeExplainsLegacyToUpstream(t *testing.T) {
	title, body := errorNotice(langRU, "install", fmt.Errorf("refusing automatic migration from legacy to upstream"))
	if title != "Операция не выполнена" || !strings.Contains(body, "чистый VDS") || !strings.Contains(body, "не изменена") {
		t.Fatalf("migration notice lacks actionable guidance: %s / %s", title, body)
	}
}

func TestDropdownRendersChoicesAndBackHint(t *testing.T) {
	view := (dropdownModel{title: "Engine", choices: []string{"legacy", "upstream"}}).View().Content
	if !strings.Contains(view, "legacy") || !strings.Contains(view, "upstream") || !strings.Contains(view, "Esc/back return") {
		t.Fatalf("dropdown view is incomplete: %s", view)
	}
}

func TestDropdownEscapeReturnsToParent(t *testing.T) {
	model := dropdownModel{choices: []string{"legacy", "upstream"}}
	updated, command := model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	model = updated.(dropdownModel)
	if command == nil || !model.cancelled {
		t.Fatalf("dropdown did not signal back navigation: model=%+v command=%v", model, command)
	}
}

func TestPromptBackReturnsNavigationSignal(t *testing.T) {
	_, err := prompt(bufio.NewReader(strings.NewReader("back\n")), &strings.Builder{}, "Host", "")
	if !errors.Is(err, errTUIBack) {
		t.Fatalf("back command did not return navigation signal: %v", err)
	}
}
