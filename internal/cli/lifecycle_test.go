package cli

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/chelslava/amneziawg-vds-setup/v2/internal/config"
	"github.com/chelslava/amneziawg-vds-setup/v2/internal/state"
)

type scriptedRunner struct {
	statePayload string
	healthCalls  int
	commands     []string
}

func (r *scriptedRunner) Run(_ context.Context, command string) (string, error) {
	r.commands = append(r.commands, command)
	switch {
	case strings.Contains(command, "if test -f /opt/awg-vds/install-state.json"):
		return r.statePayload, nil
	case strings.Contains(command, "BACKUP_PATH"):
		return "BACKUP_PATH=/opt/awg-vds/backups/awg-vds-20260725.tar.gz\nBACKUP_SHA256=abc123\n", nil
	case strings.Contains(command, "tar -C /opt/awg-vds -xzf"):
		return "RESTORED=/opt/awg-vds/backups/awg-vds-20260725.tar.gz\n", nil
	case strings.Contains(command, "HEALTH=ok"):
		r.healthCalls++
		if r.healthCalls == 1 {
			return "", errors.New("health failed")
		}
		return "HEALTH=ok\n", nil
	case strings.Contains(command, "PREFLIGHT=ok"):
		return "PREFLIGHT=ok\n", nil
	case strings.Contains(command, "DEPENDENCIES=ok"):
		return "DEPENDENCIES=ok\n", nil
	case strings.Contains(command, "CONFIG=preserved"):
		return "CONFIG=preserved\n", nil
	case strings.Contains(command, "FIREWALL="):
		return "FIREWALL=not-configured\n", nil
	case strings.Contains(command, "install-state.json.tmp"):
		return "", nil
	default:
		return "ok\n", nil
	}
}

func encodedStatePayload(t *testing.T, s state.State) string {
	t.Helper()
	b, err := state.Encode(s)
	if err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(b)
}

func TestInstallPromotesStateOnlyAfterHealth(t *testing.T) {
	runner := &scriptedRunner{}
	err := install(context.Background(), runner, config.Options{Engine: config.Legacy, Host: "192.0.2.1", User: "root", SSHPort: 22, VPNPort: 1234, WebPort: 51821}, &strings.Builder{})
	if err == nil || !strings.Contains(err.Error(), "post-install health") {
		t.Fatalf("expected post-install health failure, got %v", err)
	}
	for _, command := range runner.commands {
		if strings.Contains(command, "install-state.json.tmp") {
			t.Fatal("install promoted state before health succeeded")
		}
	}
}

func TestUpdateRestoresSnapshotAfterHealthFailure(t *testing.T) {
	s := state.State{Version: 1, Engine: config.Legacy, Image: "ghcr.io/yokitoki/awg-easy:1.0.1", Container: "awg-vds-legacy", VPNPort: 1234, WebPort: 51821, TLSMode: "disabled", ConfigPath: "/opt/awg-vds/wireguard", BackupPath: "/opt/awg-vds/backups", InstalledAt: time.Unix(1, 0).UTC(), UpdatedAt: time.Unix(1, 0).UTC()}
	runner := &scriptedRunner{statePayload: encodedStatePayload(t, s)}
	err := existing(context.Background(), runner, config.Options{}, &strings.Builder{}, "update")
	if err == nil || !strings.Contains(err.Error(), "post-update health") {
		t.Fatalf("expected post-update health failure, got %v", err)
	}
	backupIndex, restoreIndex := -1, -1
	updateCount := 0
	for i, command := range runner.commands {
		switch {
		case strings.Contains(command, "BACKUP_PATH"):
			backupIndex = i
		case strings.Contains(command, "tar -C /opt/awg-vds -xzf"):
			restoreIndex = i
		case strings.Contains(command, "docker pull"):
			updateCount++
		}
	}
	if backupIndex < 0 || restoreIndex < 0 || restoreIndex <= backupIndex || updateCount < 2 {
		t.Fatalf("update did not backup, restore, and redeploy in order: %#v", runner.commands)
	}
	for _, command := range runner.commands {
		if strings.Contains(command, "install-state.json.tmp") {
			t.Fatal("failed update promoted new state")
		}
	}
}
