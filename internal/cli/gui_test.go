package cli

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGUIBuildArgsInstall(t *testing.T) {
	args, err := guiBuildArgs(&guiRequest{Action: "install", Host: "198.51.100.7", SSHPort: 2222, User: "root", Auth: "key", Identity: "~/.ssh/id_ed25519", Engine: "upstream", VPNPort: 1234, WebPort: 51821, RestrictIP: "95.25.41.234", Timeout: 1800})
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(args, " ")
	for _, want := range []string{"install", "--host 198.51.100.7", "--ssh-port 2222", "--user root", "--identity-file ~/.ssh/id_ed25519", "--engine upstream", "--vpn-port 1234", "--web-port 51821", "--restrict-panel-ip 95.25.41.234", "--timeout 1800"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("gui install args lack %q: %s", want, joined)
		}
	}
}

func TestGUIBuildArgsDefaultsAndTLSMatrix(t *testing.T) {
	args, err := guiBuildArgs(&guiRequest{Action: "install", Host: "198.51.100.7", Engine: "upstream"})
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(args, " ")
	for _, want := range []string{"--ssh-port 22", "--user root", "--engine upstream", "--vpn-port 1234", "--web-port 51821"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("gui defaults missing %q: %s", want, joined)
		}
	}
	if _, err := guiBuildArgs(&guiRequest{Action: "install", Host: "h", TLS: true}); err == nil {
		t.Fatal("tls without domain must be rejected")
	}
	if _, err := guiBuildArgs(&guiRequest{Action: "install", Host: "h", Domain: "vpn.example.com"}); err == nil {
		t.Fatal("domain without tls must be rejected")
	}
	if _, err := guiBuildArgs(&guiRequest{Action: "install", Host: "h", Domain: "vpn.example.com", TLS: true, Engine: "upstream"}); err != nil {
		t.Fatalf("domain+tls rejected unexpectedly: %v", err)
	}
}

func TestGUIBuildArgsRejectsBadInput(t *testing.T) {
	if _, err := guiBuildArgs(&guiRequest{Action: "install", Host: ""}); err == nil {
		t.Fatal("empty host must be rejected")
	}
	if _, err := guiBuildArgs(&guiRequest{Action: "install", Host: "a b"}); err == nil {
		t.Fatal("host with whitespace must be rejected")
	}
	if _, err := guiBuildArgs(&guiRequest{Action: "reboot-server", Host: "h"}); err == nil {
		t.Fatal("unknown action must be rejected")
	}
	if _, err := guiBuildArgs(&guiRequest{Action: "status", Host: "h", SSHPort: 70000}); err == nil {
		t.Fatal("invalid ssh port must be rejected")
	}
}

func TestGUIServerEnforcesTokenAndServesPage(t *testing.T) {
	srv := newGUIServer("tok123", nil)
	rec := httptest.NewRecorder()
	srv.handlePage(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("page without token must be forbidden, got %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	srv.handlePage(rec, httptest.NewRequest(http.MethodGet, "/?token=tok123", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("page with token must be ok, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "awg-vds") || !strings.Contains(rec.Body.String(), "Движок") || !strings.Contains(rec.Body.String(), "Журнал операции") {
		t.Fatalf("gui page content is incomplete")
	}
	rec = httptest.NewRecorder()
	srv.handleRun(rec, httptest.NewRequest(http.MethodPost, "/api/run", strings.NewReader("{}")))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("run without token must be forbidden, got %d", rec.Code)
	}
}

func TestGUIJobLifecycleRunsCommandAndCapturesOutput(t *testing.T) {
	runner := func(ctx context.Context, args []string, out, errOut io.Writer, prompt thirdPartyRepoPrompt, password string) error {
		_, _ = out.Write([]byte("HEALTH=ok\n"))
		if password != "" {
			_, _ = out.Write([]byte("password-was-set"))
		}
		return errors.New("boom")
	}
	srv := newGUIServer("tok", guiRunner(runner))
	rec := httptest.NewRecorder()
	srv.handleRun(rec, httptest.NewRequest(http.MethodPost, "/api/run?token=tok", strings.NewReader(`{"action":"status","host":"198.51.100.7","auth":"password","password":"secret"}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("run failed: %d %s", rec.Code, rec.Body.String())
	}
	var started map[string]string
	_ = json.Unmarshal(rec.Body.Bytes(), &started)
	if started["job"] == "" {
		t.Fatal("run did not return a job id")
	}
	deadline := 50
	for i := 0; i < deadline; i++ {
		rec = httptest.NewRecorder()
		srv.handleJob(rec, httptest.NewRequest(http.MethodGet, "/api/job?token=tok", nil))
		var state struct {
			Running bool   `json:"running"`
			Done    bool   `json:"done"`
			Error   string `json:"error"`
			Output  string `json:"output"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &state)
		if state.Done {
			if state.Running {
				t.Fatal("done job must not be running")
			}
			if !strings.Contains(state.Output, "HEALTH=ok") || !strings.Contains(state.Output, "password-was-set") {
				t.Fatalf("job output is incomplete: %q", state.Output)
			}
			if state.Error != "boom" {
				t.Fatalf("job error mismatch: %q", state.Error)
			}
			return
		}
	}
	t.Fatal("job never completed")
}

func TestGUIRejectsNonLoopbackListen(t *testing.T) {
	var out, errOut strings.Builder
	err := guiCommand([]string{"--listen", "0.0.0.0:8620"}, &out, &errOut)
	if err == nil || !strings.Contains(err.Error(), "loopback") {
		t.Fatalf("non-loopback bind must be rejected, got %v", err)
	}
}
