package config

import (
	"strings"
	"testing"
)

func TestValidateInstallTLSMatrix(t *testing.T) {
	tests := []struct {
		name   string
		domain string
		tls    bool
		want   string
	}{
		{name: "host only", domain: "", tls: false},
		{name: "domain with tls", domain: "vpn.example.com", tls: true},
		{name: "domain without tls", domain: "vpn.example.com", tls: false, want: "requires --tls"},
		{name: "tls without domain", domain: "", tls: true, want: "requires --domain"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o := Options{Host: "192.0.2.1", User: "root", SSHPort: 22, Engine: Legacy, VPNPort: 1234, WebPort: 51821, Domain: tt.domain, TLS: tt.tls}
			err := o.ValidateInstall()
			if tt.want == "" && err != nil {
				t.Fatalf("unexpected validation error: %v", err)
			}
			if tt.want != "" && (err == nil || !strings.Contains(err.Error(), tt.want)) {
				t.Fatalf("expected %q, got %v", tt.want, err)
			}
		})
	}
}
