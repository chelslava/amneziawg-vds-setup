package state

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/chelslava/amneziawg-vds-setup/v2/internal/config"
)

const Path = "/opt/awg-vds/install-state.json"

type State struct {
	Version          int           `json:"schema_version"`
	Engine           config.Engine `json:"engine"`
	Image            string        `json:"image"`
	Container        string        `json:"container"`
	VPNPort          int           `json:"vpn_port"`
	WebPort          int           `json:"web_port"`
	Domain           string        `json:"domain,omitempty"`
	TLSMode          string        `json:"tls_mode"`
	ConfigPath       string        `json:"config_path"`
	BackupPath       string        `json:"backup_path"`
	LastBackupPath   string        `json:"last_backup_path,omitempty"`
	LastBackupSHA256 string        `json:"last_backup_sha256,omitempty"`
	InstalledAt      time.Time     `json:"installed_at"`
	UpdatedAt        time.Time     `json:"updated_at"`
}

func (s State) Validate() error {
	if s.Version != 1 {
		return fmt.Errorf("unsupported state schema %d", s.Version)
	}
	if s.Engine != config.Legacy && s.Engine != config.Upstream {
		return fmt.Errorf("invalid engine %q", s.Engine)
	}
	if s.Image == "" || s.Container == "" || s.ConfigPath == "" || s.BackupPath == "" {
		return fmt.Errorf("state is missing required fields")
	}
	if s.VPNPort < 1 || s.VPNPort > 65535 || s.WebPort < 1 || s.WebPort > 65535 || s.VPNPort == s.WebPort {
		return fmt.Errorf("invalid state ports")
	}
	if s.TLSMode != "disabled" && s.TLSMode != "caddy" {
		return fmt.Errorf("invalid TLS mode %q", s.TLSMode)
	}
	if (s.TLSMode == "caddy") != (s.Domain != "") {
		return fmt.Errorf("domain and TLS mode are inconsistent")
	}
	if (s.LastBackupPath == "") != (s.LastBackupSHA256 == "") {
		return fmt.Errorf("backup metadata is incomplete")
	}
	return nil
}

func Encode(s State) ([]byte, error) {
	if err := s.Validate(); err != nil {
		return nil, err
	}
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetIndent("", "  ")
	if err := enc.Encode(s); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

func Decode(r io.Reader) (State, error) {
	var s State
	dec := json.NewDecoder(r)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&s); err != nil {
		return State{}, fmt.Errorf("decode install state: %w", err)
	}
	if err := s.Validate(); err != nil {
		return State{}, err
	}
	return s, nil
}
