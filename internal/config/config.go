package config

import (
	"fmt"
	"net"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

type Engine string

const (
	Legacy   Engine = "legacy"
	Upstream Engine = "upstream"

	LegacyImage         = "ghcr.io/yokitoki/awg-easy@sha256:bfb9070d88379dc31ce55ef5588915964a2c3abd657249c696dd375202df3f6f"
	LegacyImageOldTag   = "ghcr.io/yokitoki/awg-easy:1.0.1"
	UpstreamImage       = "ghcr.io/wg-easy/wg-easy@sha256:4ffc03c35dce5456bbb2fa6b136a1eeb196394548dee0650ae692efdd1062e01"
	UpstreamImageOldTag = "ghcr.io/wg-easy/wg-easy:15.2.1"
)

type Options struct {
	Command     string
	Host        string
	SSHPort     int
	User        string
	Identity    string
	KnownHosts  string
	Engine      Engine
	VPNPort     int
	WebPort     int
	Domain      string
	TLS         bool
	RestrictIP  string
	Force       bool
	TimeoutSecs int
}

func (o Options) Address() string { return net.JoinHostPort(o.Host, strconv.Itoa(o.SSHPort)) }

func (o Options) ValidateConnection() error {
	if strings.TrimSpace(o.Host) == "" {
		return fmt.Errorf("--host is required")
	}
	if strings.ContainsAny(o.Host, "\r\n \t") || strings.ContainsAny(o.User, "\r\n \t") {
		return fmt.Errorf("host and user must not contain whitespace")
	}
	if !regexp.MustCompile(`^[a-z_][a-z0-9_-]*$`).MatchString(o.User) {
		return fmt.Errorf("invalid SSH user %q", o.User)
	}
	if o.SSHPort < 1 || o.SSHPort > 65535 {
		return fmt.Errorf("SSH port must be between 1 and 65535")
	}
	if o.Identity != "" {
		o.Identity = filepath.Clean(o.Identity)
	}
	if o.KnownHosts != "" {
		o.KnownHosts = filepath.Clean(o.KnownHosts)
	}
	return nil
}

func (o Options) ValidateInstall() error {
	if err := o.ValidateConnection(); err != nil {
		return err
	}
	if o.Engine != Legacy && o.Engine != Upstream {
		return fmt.Errorf("engine must be legacy or upstream")
	}
	if o.VPNPort < 1 || o.VPNPort > 65535 || o.WebPort < 1 || o.WebPort > 65535 || o.VPNPort == o.WebPort {
		return fmt.Errorf("VPN and web ports must be distinct values between 1 and 65535")
	}
	if o.Domain != "" {
		if net.ParseIP(o.Domain) != nil || !regexp.MustCompile(`^(?i:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?)(?:\.(?i:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?))+$`).MatchString(o.Domain) {
			return fmt.Errorf("--domain must be a DNS name, not an IP address")
		}
	}
	if o.TLS && o.Domain == "" {
		return fmt.Errorf("--tls requires --domain")
	}
	if o.Domain != "" && !o.TLS {
		return fmt.Errorf("--domain requires --tls; use --tls or omit --domain")
	}
	if o.RestrictIP != "" && net.ParseIP(o.RestrictIP) == nil {
		return fmt.Errorf("--restrict-panel-ip must be an IP address")
	}
	return nil
}
