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

type matcherRunner struct {
	commands []string
	match    func(string) (string, error)
}

func (r *matcherRunner) Run(_ context.Context, command string) (string, error) {
	r.commands = append(r.commands, command)
	return r.match(command)
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
	s := state.State{Version: 1, Engine: config.Legacy, Image: "ghcr.io/yokitoki/awg-easy@sha256:bfb9070d88379dc31ce55ef5588915964a2c3abd657249c696dd375202df3f6f", Container: "awg-vds-legacy", VPNPort: 1234, WebPort: 51821, TLSMode: "disabled", ConfigPath: "/opt/awg-vds/wireguard", BackupPath: "/opt/awg-vds/backups", InstalledAt: time.Unix(1, 0).UTC(), UpdatedAt: time.Unix(1, 0).UTC()}
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

func TestInstallRejectsBusyPortBeforeMutatingEmptyHost(t *testing.T) {
	runner := &matcherRunner{match: func(command string) (string, error) {
		if strings.Contains(command, "if test -f /opt/awg-vds/install-state.json") {
			return "", nil
		}
		return "PORT_TCP_51821=busy\nPREFLIGHT=ok\n", nil
	}}
	err := install(context.Background(), runner, config.Options{Engine: config.Legacy, Host: "192.0.2.1", User: "root", SSHPort: 22, VPNPort: 1234, WebPort: 51821}, &strings.Builder{})
	if err == nil || !strings.Contains(err.Error(), "already occupied") {
		t.Fatalf("expected busy-port refusal, got %v", err)
	}
	if len(runner.commands) != 2 {
		t.Fatalf("install mutated host after busy preflight: %#v", runner.commands)
	}
}

func TestUpstreamInstallRejectsUnsupportedModule(t *testing.T) {
	runner := &matcherRunner{match: func(command string) (string, error) {
		if strings.Contains(command, "if test -f /opt/awg-vds/install-state.json") {
			return "", nil
		}
		return "AMNEZIAWG=unsupported\nPREFLIGHT=ok\n", nil
	}}
	err := install(context.Background(), runner, config.Options{Engine: config.Upstream, Host: "192.0.2.1", User: "root", SSHPort: 22, VPNPort: 1234, WebPort: 51821}, &strings.Builder{})
	if err == nil || !strings.Contains(err.Error(), "requires the AmneziaWG kernel module") {
		t.Fatalf("expected upstream module refusal, got %v", err)
	}
}

func TestExistingCommandRequiresState(t *testing.T) {
	runner := &matcherRunner{match: func(command string) (string, error) {
		if strings.Contains(command, "if test -f /opt/awg-vds/install-state.json") {
			return "", nil
		}
		return "ok", nil
	}}
	err := existing(context.Background(), runner, config.Options{}, &strings.Builder{}, "status")
	if err == nil || !strings.Contains(err.Error(), "no v2 installation state") {
		t.Fatalf("expected missing-state error, got %v", err)
	}
}

func TestSameEngineInstallReconcilesExpectedBusyPort(t *testing.T) {
	s := state.State{Version: 1, Engine: config.Legacy, Image: config.LegacyImage, Container: "awg-vds-legacy", VPNPort: 1234, WebPort: 51821, TLSMode: "disabled", ConfigPath: "/opt/awg-vds/wireguard", BackupPath: "/opt/awg-vds/backups", InstalledAt: time.Unix(1, 0).UTC(), UpdatedAt: time.Unix(1, 0).UTC()}
	runner := &matcherRunner{match: func(command string) (string, error) {
		switch {
		case strings.Contains(command, "if test -f /opt/awg-vds/install-state.json"):
			return encodedStatePayload(t, s), nil
		case strings.Contains(command, "PORT_TCP_51821"):
			return "PORT_TCP_51821=busy\nPREFLIGHT=ok\n", nil
		case strings.Contains(command, "HEALTH=ok"):
			return "HEALTH=ok\n", nil
		default:
			return "ok\n", nil
		}
	}}
	var out strings.Builder
	if err := install(context.Background(), runner, config.Options{Engine: config.Legacy, Host: "192.0.2.1", User: "root", SSHPort: 22, VPNPort: 1234, WebPort: 51821}, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "reconciling") || !strings.Contains(out.String(), "plain HTTP") {
		t.Fatalf("reconcile summary is incomplete: %s", out.String())
	}
}

func TestRotatePasswordBacksUpAndCleansOnlyAfterHealth(t *testing.T) {
	s := state.State{Version: 1, Engine: config.Legacy, Image: config.LegacyImage, Container: "awg-vds-legacy", VPNPort: 1234, WebPort: 51821, TLSMode: "disabled", ConfigPath: "/opt/awg-vds/wireguard", BackupPath: "/opt/awg-vds/backups", InstalledAt: time.Unix(1, 0).UTC(), UpdatedAt: time.Unix(1, 0).UTC()}
	runner := &matcherRunner{match: func(command string) (string, error) {
		switch {
		case strings.Contains(command, "if test -f /opt/awg-vds/install-state.json"):
			return encodedStatePayload(t, s), nil
		case strings.Contains(command, "BACKUP_PATH"):
			return "BACKUP_PATH=/opt/awg-vds/backups/panel-rotation.tar.gz\nBACKUP_SHA256=abc123\n", nil
		case strings.Contains(command, "ROTATION=prepared"):
			return "ROTATION=prepared\n", nil
		case strings.Contains(command, "HEALTH=ok"):
			return "HEALTH=ok\n", nil
		case strings.Contains(command, "ROTATION=cleaned"):
			return "ROTATION=cleaned\n", nil
		default:
			return "", nil
		}
	}}
	var out strings.Builder
	if err := existing(context.Background(), runner, config.Options{}, &out, "rotate-password"); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(runner.commands, "\n")
	if !strings.Contains(joined, "BACKUP_PATH") || !strings.Contains(joined, "docker restart 'awg-vds-legacy'") || !strings.Contains(joined, "rm -f /opt/awg-vds/.panel-rotation") {
		t.Fatalf("rotation lifecycle is incomplete: %s", joined)
	}
	if strings.Contains(out.String(), "PASSWORD_HASH=") || strings.Contains(out.String(), "ROTATION_PASSWORD=") {
		t.Fatalf("rotation output exposed secret material: %s", out.String())
	}
}

func TestRotatePasswordRollsBackAfterHealthFailure(t *testing.T) {
	s := state.State{Version: 1, Engine: config.Legacy, Image: config.LegacyImage, Container: "awg-vds-legacy", VPNPort: 1234, WebPort: 51821, TLSMode: "disabled", ConfigPath: "/opt/awg-vds/wireguard", BackupPath: "/opt/awg-vds/backups", InstalledAt: time.Unix(1, 0).UTC(), UpdatedAt: time.Unix(1, 0).UTC()}
	runner := &matcherRunner{match: func(command string) (string, error) {
		switch {
		case strings.Contains(command, "if test -f /opt/awg-vds/install-state.json"):
			return encodedStatePayload(t, s), nil
		case strings.Contains(command, "BACKUP_PATH"):
			return "BACKUP_PATH=/opt/awg-vds/backups/panel-rotation.tar.gz\nBACKUP_SHA256=abc123\n", nil
		case strings.Contains(command, "ROTATION=prepared"):
			return "ROTATION=prepared\n", nil
		case strings.Contains(command, "HEALTH=ok"):
			return "", errors.New("panel unavailable")
		case strings.Contains(command, "ROTATION=rolled-back"):
			return "ROTATION=rolled-back\n", nil
		default:
			return "", nil
		}
	}}
	if err := existing(context.Background(), runner, config.Options{}, &strings.Builder{}, "rotate-password"); err == nil || !strings.Contains(err.Error(), "previous credential restored") {
		t.Fatalf("expected health rollback, got %v", err)
	}
	joined := strings.Join(runner.commands, "\n")
	if !strings.Contains(joined, "ROTATION=rolled-back") || strings.Contains(joined, "ROTATION=cleaned") {
		t.Fatalf("rollback ordering is unsafe: %s", joined)
	}
}
