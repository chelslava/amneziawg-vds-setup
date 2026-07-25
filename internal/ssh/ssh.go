package ssh

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/charmbracelet/x/term"
	"github.com/chelslava/amneziawg-vds-setup/v2/internal/config"
)

type Client struct {
	Options     config.Options
	Stdin       io.Reader
	Stdout      io.Writer
	Stderr      io.Writer
	ControlPath string
	password    string
	passwordSet bool
}

// ForgetPassword clears the in-memory password after an operation completes.
func (c *Client) ForgetPassword() {
	password := []byte(c.password)
	clear(password)
	c.password = ""
	c.passwordSet = false
}

func (c Client) baseArgs() []string {
	a := []string{"-p", fmt.Sprint(c.Options.SSHPort), "-o", "ConnectTimeout=15", "-o", "BatchMode=no", "-o", "StrictHostKeyChecking=yes"}
	if c.Options.KnownHosts != "" {
		a = append(a, "-o", "UserKnownHostsFile="+c.Options.KnownHosts)
	}
	if c.Options.Identity != "" {
		a = append(a, "-i", c.Options.Identity, "-o", "IdentitiesOnly=yes")
	} else {
		// Password is collected once, interactively, and supplied to OpenSSH via
		// the in-memory askpass helper configured in commandEnv.
		a = append(a, "-o", "PreferredAuthentications=publickey,password,keyboard-interactive", "-o", "PasswordAuthentication=yes", "-o", "KbdInteractiveAuthentication=yes")
	}
	if c.ControlPath != "" {
		a = append(a, "-o", "ControlMaster=auto", "-o", "ControlPersist=120s", "-o", "ControlPath="+c.ControlPath)
	}
	return a
}

func (c Client) args() []string {
	return append(c.baseArgs(), c.Options.User+"@"+c.Options.Host)
}

// EnableConnectionReuse creates a temporary OpenSSH control socket. The first
// command authenticates interactively; subsequent commands reuse that session.
func (c *Client) EnableConnectionReuse() (func(), error) {
	// Win32-OpenSSH does not implement Unix-domain ControlPath sockets. Passing
	// these options on Windows causes "getsockname failed: Not a socket".
	if runtime.GOOS == "windows" {
		return func() {}, nil
	}
	dir, err := os.MkdirTemp("", "awg-vds-ssh-")
	if err != nil {
		return nil, err
	}
	c.ControlPath = filepath.Join(dir, "control")
	cleanup := func() {
		args := append(c.baseArgs(), "-O", "exit", c.Options.User+"@"+c.Options.Host)
		cmd := exec.Command("ssh", args...)
		cmd.Stdin = strings.NewReader("")
		cmd.Stdout = io.Discard
		cmd.Stderr = io.Discard
		_ = cmd.Run()
		_ = os.RemoveAll(dir)
	}
	return cleanup, nil
}

func (c *Client) Run(ctx context.Context, command string) (string, error) {
	if err := c.ensurePassword(); err != nil {
		return "", err
	}
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
	cmd.Env = c.commandEnv()
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

func (c *Client) ensurePassword() error {
	if c.Options.Identity != "" || c.passwordSet {
		return nil
	}
	if !term.IsTerminal(os.Stdin.Fd()) {
		return fmt.Errorf("SSH password requires an interactive terminal; use --identity-file for non-interactive runs")
	}
	w := c.Stderr
	if w == nil {
		w = os.Stderr
	}
	_, _ = fmt.Fprint(w, "SSH password (entered once): ")
	password, err := term.ReadPassword(os.Stdin.Fd())
	_, _ = fmt.Fprintln(w)
	if err != nil {
		return fmt.Errorf("read SSH password: %w", err)
	}
	c.password = string(password)
	c.passwordSet = true
	return nil
}

func (c *Client) commandEnv() []string {
	if c.Options.Identity != "" || !c.passwordSet {
		return nil
	}
	executable, err := os.Executable()
	if err != nil {
		return nil
	}
	env := append([]string{}, os.Environ()...)
	env = append(env,
		"SSH_ASKPASS="+executable,
		"SSH_ASKPASS_REQUIRE=force",
		"DISPLAY=awg-vds",
		"AWG_VDS_ASKPASS=1",
		"AWG_VDS_SSH_PASSWORD="+c.password,
	)
	return env
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
