package ssh

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/chelslava/amneziawg-vds-setup/v2/internal/config"
)

type Client struct {
	Options config.Options
	Stdin   io.Reader
	Stdout  io.Writer
	Stderr  io.Writer
}

func (c Client) args() []string {
	a := []string{"-p", fmt.Sprint(c.Options.SSHPort), "-o", "ConnectTimeout=15", "-o", "BatchMode=no", "-o", "StrictHostKeyChecking=yes"}
	if c.Options.KnownHosts != "" {
		a = append(a, "-o", "UserKnownHostsFile="+c.Options.KnownHosts)
	}
	if c.Options.Identity != "" {
		a = append(a, "-i", c.Options.Identity, "-o", "IdentitiesOnly=yes")
	}
	return append(a, c.Options.User+"@"+c.Options.Host)
}

func (c Client) Run(ctx context.Context, command string) (string, error) {
	timeout := time.Duration(c.Options.TimeoutSecs) * time.Second
	if timeout <= 0 {
		timeout = 2 * time.Minute
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	args := append(c.args(), command)
	cmd := exec.CommandContext(ctx, "ssh", args...)
	cmd.Stdin = c.Stdin
	if cmd.Stdin == nil {
		cmd.Stdin = os.Stdin
	}
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if c.Stderr == nil {
		c.Stderr = os.Stderr
	}
	if err := cmd.Run(); err != nil {
		writeRedacted(c.Stderr, stderr.String())
		if ctx.Err() != nil {
			return "", fmt.Errorf("SSH command timed out: %w", ctx.Err())
		}
		detail := strings.TrimSpace(sanitize(stderr.String()))
		if detail != "" {
			return sanitize(stdout.String()), fmt.Errorf("SSH command failed: %w: %s", err, detail)
		}
		return sanitize(stdout.String()), fmt.Errorf("SSH command failed: %w", err)
	}
	writeRedacted(c.Stderr, stderr.String())
	return sanitize(stdout.String()), nil
}

var secretLine = regexp.MustCompile(`(?im)(password_hash|panel_password|private_key|preshared_key|ssh_password)\s*[:=]\s*[^\r\n]+`)
var secretValue = regexp.MustCompile(`(?i)(password|private[_ -]?key|preshared[_ -]?key)\s*[:=]\s*\S+`)

func sanitize(s string) string {
	s = secretLine.ReplaceAllString(s, "$1=[REDACTED]")
	s = secretValue.ReplaceAllString(s, "$1=[REDACTED]")
	return s
}

func Redact(s string) string { return sanitize(strings.TrimSpace(s)) }

func PrintOutput(w io.Writer, output string) {
	if w == nil {
		return
	}
	_, _ = io.Copy(w, strings.NewReader(sanitize(output)))
}

func writeRedacted(w io.Writer, output string) {
	if w == nil || output == "" {
		return
	}
	_, _ = io.WriteString(w, sanitize(output))
}
