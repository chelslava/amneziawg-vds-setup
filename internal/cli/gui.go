package cli

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/chelslava/amneziawg-vds-setup/v2/internal/ssh"
)

type guiRequest struct {
	Action     string `json:"action"`
	Host       string `json:"host"`
	SSHPort    int    `json:"ssh_port"`
	User       string `json:"user"`
	Auth       string `json:"auth"`
	Identity   string `json:"identity"`
	KnownHosts string `json:"known_hosts"`
	Engine     string `json:"engine"`
	VPNPort    int    `json:"vpn_port"`
	WebPort    int    `json:"web_port"`
	Domain     string `json:"domain"`
	TLS        bool   `json:"tls"`
	RestrictIP string `json:"restrict_ip"`
	Timeout    int    `json:"timeout"`
	Password   string `json:"password"`
}

type guiJob struct {
	id        string
	mu        sync.Mutex
	output    strings.Builder
	done      bool
	errText   string
	cancel    context.CancelFunc
	startedAt time.Time
}

func (j *guiJob) Write(p []byte) (int, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.output.Write(p)
}

func (j *guiJob) snapshot() (string, bool, string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.output.String(), j.done, j.errText
}

func (j *guiJob) StreamedOutput() bool { return true }

type guiRunner func(ctx context.Context, args []string, out, errOut io.Writer, prompt thirdPartyRepoPrompt, password string) error

type guiServer struct {
	token    string
	runner   guiRunner
	mu       sync.Mutex
	current  *guiJob
	lastDone *guiJob
}

func newGUIServer(token string, runner guiRunner) *guiServer {
	if runner == nil {
		runner = runCommandWithPromptContext
	}
	return &guiServer{token: token, runner: runner}
}

func guiToken() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		panic(fmt.Sprintf("generate gui token: %v", err))
	}
	return hex.EncodeToString(buf)
}

func guiCommand(args []string, out, errOut io.Writer) error {
	fs := flag.NewFlagSet("gui", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	listen := fs.String("listen", "127.0.0.1:8620", "local listen address (loopback only)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}
	host, port, err := net.SplitHostPort(*listen)
	if err != nil {
		return fmt.Errorf("invalid --listen %q: %w", *listen, err)
	}
	if !guiIsLoopback(host) {
		return fmt.Errorf("gui must bind to a loopback address (127.0.0.1, ::1, localhost); got %q", host)
	}
	token := guiToken()
	srv := newGUIServer(token, nil)
	mux := http.NewServeMux()
	mux.HandleFunc("/", srv.handlePage)
	mux.HandleFunc("/api/run", srv.handleRun)
	mux.HandleFunc("/api/job", srv.handleJob)
	mux.HandleFunc("/api/cancel", srv.handleCancel)
	addr := net.JoinHostPort(host, port)
	fmt.Fprintf(out, "awg-vds GUI: http://%s/?token=%s\n", addr, token)
	fmt.Fprintln(out, "Останов: Ctrl+C. Сервер слушает только loopback; токен обязателен.")
	server := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	return server.ListenAndServe()
}

func guiIsLoopback(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func (s *guiServer) authorized(r *http.Request) bool {
	return r.URL.Query().Get("token") == s.token
}

func (s *guiServer) handlePage(w http.ResponseWriter, r *http.Request) {
	if !s.authorized(r) {
		http.Error(w, "403: откройте GUI по ссылке с токеном из вывода awg-vds gui", http.StatusForbidden)
		return
	}
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = io.WriteString(w, guiHTML)
}

func (s *guiServer) handleRun(w http.ResponseWriter, r *http.Request) {
	if !s.authorized(r) {
		http.Error(w, "403 forbidden", http.StatusForbidden)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "405 method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req guiRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "400 invalid json: "+err.Error(), http.StatusBadRequest)
		return
	}
	args, err := guiBuildArgs(&req)
	if err != nil {
		http.Error(w, "400 "+err.Error(), http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	if s.current != nil {
		s.mu.Unlock()
		http.Error(w, "409 operation already running", http.StatusConflict)
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	job := &guiJob{id: guiToken()[:12], cancel: cancel, startedAt: time.Now()}
	s.current = job
	s.mu.Unlock()

	password := ""
	if req.Auth == "password" {
		password = req.Password
	}
	go func() {
		err := s.runner(ctx, args, job, job, nil, password)
		job.mu.Lock()
		job.done = true
		if err != nil {
			job.errText = sshRedactForGUI(err.Error())
		}
		job.mu.Unlock()
		s.mu.Lock()
		if s.current == job {
			s.current = nil
			s.lastDone = job
		}
		s.mu.Unlock()
	}()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"job": job.id})
}

// sshRedactForGUI keeps credential material out of GUI error text.
func sshRedactForGUI(text string) string {
	return ssh.Redact(text)
}

func (s *guiServer) handleJob(w http.ResponseWriter, r *http.Request) {
	if !s.authorized(r) {
		http.Error(w, "403 forbidden", http.StatusForbidden)
		return
	}
	s.mu.Lock()
	job := s.current
	if job == nil {
		job = s.lastDone
	}
	s.mu.Unlock()
	if job == nil {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"running": false})
		return
	}
	output, done, errText := job.snapshot()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"job": job.id, "running": !done, "done": done, "error": errText, "output": output, "elapsed_s": int(time.Since(job.startedAt).Seconds())})
}

func (s *guiServer) handleCancel(w http.ResponseWriter, r *http.Request) {
	if !s.authorized(r) {
		http.Error(w, "403 forbidden", http.StatusForbidden)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "405 method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.mu.Lock()
	job := s.current
	s.mu.Unlock()
	if job == nil {
		http.Error(w, "409 no running operation", http.StatusConflict)
		return
	}
	job.cancel()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"cancelled": job.id})
}

func guiBuildArgs(req *guiRequest) ([]string, error) {
	action := strings.ToLower(strings.TrimSpace(req.Action))
	switch action {
	case "install", "doctor", "status", "update", "backup", "rotate-password":
	default:
		return nil, fmt.Errorf("unknown action %q", req.Action)
	}
	host := strings.TrimSpace(req.Host)
	if host == "" || strings.ContainsAny(host, " \t\r\n") {
		return nil, errors.New("host is required and must not contain whitespace")
	}
	if req.SSHPort == 0 {
		req.SSHPort = 22
	}
	if req.SSHPort < 1 || req.SSHPort > 65535 {
		return nil, errors.New("ssh_port must be between 1 and 65535")
	}
	user := strings.TrimSpace(req.User)
	if user == "" {
		user = "root"
	}
	args := []string{action, "--host", host, "--ssh-port", fmt.Sprintf("%d", req.SSHPort), "--user", user}
	if identity := strings.TrimSpace(req.Identity); identity != "" {
		args = append(args, "--identity-file", identity)
	}
	if knownHosts := strings.TrimSpace(req.KnownHosts); knownHosts != "" {
		args = append(args, "--known-hosts", knownHosts)
	}
	if action == "install" || action == "doctor" {
		engine := strings.ToLower(strings.TrimSpace(req.Engine))
		if engine == "" {
			engine = "upstream"
		}
		if engine != "legacy" && engine != "upstream" {
			return nil, errors.New("engine must be legacy or upstream")
		}
		args = append(args, "--engine", engine)
	}
	if action == "install" {
		if req.VPNPort == 0 {
			req.VPNPort = 1234
		}
		if req.WebPort == 0 {
			req.WebPort = 51821
		}
		args = append(args, "--vpn-port", fmt.Sprintf("%d", req.VPNPort), "--web-port", fmt.Sprintf("%d", req.WebPort))
		domain := strings.TrimSpace(req.Domain)
		if req.TLS {
			if domain == "" {
				return nil, errors.New("--tls requires a domain")
			}
			args = append(args, "--domain", domain, "--tls")
		} else if domain != "" {
			return nil, errors.New("domain requires --tls")
		}
		if restrictIP := strings.TrimSpace(req.RestrictIP); restrictIP != "" {
			args = append(args, "--restrict-panel-ip", restrictIP)
		}
		if req.Timeout > 0 {
			args = append(args, "--timeout", fmt.Sprintf("%d", req.Timeout))
		}
	}
	return args, nil
}

const guiHTML = `<!doctype html>
<html lang="ru">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>awg-vds — панель управления</title>
<style>
:root { color-scheme: dark; }
* { box-sizing: border-box; }
body { font-family: "Segoe UI", system-ui, sans-serif; background: #12141a; color: #e6e9ef; margin: 0; padding: 24px; }
.wrap { max-width: 980px; margin: 0 auto; }
h1 { font-size: 20px; margin: 0 0 4px; }
.sub { color: #8a93a5; font-size: 13px; margin-bottom: 20px; }
.card { background: #1a1e27; border: 1px solid #262c3a; border-radius: 10px; padding: 18px; margin-bottom: 16px; }
.grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(220px, 1fr)); gap: 12px; }
label { display: block; font-size: 12px; color: #9aa4b8; margin-bottom: 4px; }
input, select { width: 100%; background: #12141a; color: #e6e9ef; border: 1px solid #2c3444; border-radius: 6px; padding: 8px 10px; font-size: 14px; }
input:focus, select:focus { outline: 1px solid #4f7cff; }
.row { display: flex; gap: 10px; margin-top: 14px; flex-wrap: wrap; align-items: center; }
button { background: #4f7cff; border: none; color: #fff; border-radius: 8px; padding: 10px 20px; font-size: 14px; cursor: pointer; }
button:disabled { background: #3a4152; cursor: not-allowed; }
button.secondary { background: #2c3444; }
.actions button { background: #2c3444; padding: 8px 14px; font-size: 13px; }
.actions button.active { background: #4f7cff; }
pre { background: #0c0e13; border: 1px solid #262c3a; border-radius: 8px; padding: 14px; min-height: 220px; max-height: 46vh; overflow: auto; white-space: pre-wrap; word-break: break-word; font-size: 12.5px; line-height: 1.5; }
.status { font-size: 13px; color: #9aa4b8; }
.err { color: #ff7a7a; }
.hint { font-size: 12px; color: #8a93a5; margin-top: 8px; }
#cancel { display: none; }
</style>
</head>
<body>
<div class="wrap">
<h1>awg-vds — панель управления AmneziaWG</h1>
<div class="sub">Локальный GUI. Операции выполняются на выбранном VDS по SSH; пароль хранится только в памяти.</div>

<div class="card actions">
<button data-action="install" class="active">Установка</button>
<button data-action="doctor">Диагностика</button>
<button data-action="status">Статус</button>
<button data-action="update">Обновление</button>
<button data-action="backup">Бэкап</button>
<button data-action="rotate-password">Смена пароля</button>
</div>

<div class="card">
<div class="grid">
<div><label>Движок</label>
<select id="engine"><option value="upstream" selected>upstream (последняя панель)</option><option value="legacy">legacy (WireSock)</option></select></div>
<div><label>Адрес VDS</label><input id="host" placeholder="1.2.3.4"></div>
<div><label>SSH-порт</label><input id="ssh_port" value="22"></div>
<div><label>SSH-пользователь</label><input id="user" value="root"></div>
<div><label>Метод входа</label>
<select id="auth"><option value="key">Ключ</option><option value="password">Пароль</option></select></div>
<div><label>Файл ключа (путь)</label><input id="identity" placeholder="~/.ssh/id_ed25519"></div>
<div id="pw"><label>Пароль SSH (в памяти, не сохраняется)</label><input id="password" type="password"></div>
<div><label>known_hosts (пусто = системный)</label><input id="known_hosts"></div>
<div><label>VPN UDP-порт</label><input id="vpn_port" value="1234"></div>
<div><label>Панель TCP-порт</label><input id="web_port" value="51821"></div>
<div><label>Домен (только с TLS)</label><input id="domain"></div>
<div><label>TLS через Caddy</label><select id="tls"><option value="false" selected>нет</option><option value="true">да (нужен домен)</option></select></div>
<div><label>Ограничение панели по IP</label><input id="restrict_ip" placeholder="ваш IP"></div>
<div><label>Timeout SSH, сек (0 = авто)</label><input id="timeout" value="0"></div>
</div>
<div class="row">
<button id="start">Запустить</button>
<button id="cancel" class="secondary">Отменить операцию</button>
<span class="status" id="status">Готово к запуску.</span>
</div>
<div class="hint">install: VPN UDP 1234, панель TCP 51821 по умолчанию. После установки пароль панели — на сервере в /opt/awg-vds/panel-password (см. README). Панель открывается на http://VDS:51821, логин admin.</div>
</div>

<div class="card">
<label>Журнал операции</label>
<pre id="log">— пусто —</pre>
</div>
</div>

<script>
let action = "install";
let timer = null;
const $ = (id) => document.getElementById(id);
document.querySelectorAll(".actions button").forEach(b => {
  b.addEventListener("click", () => {
    document.querySelectorAll(".actions button").forEach(x => x.classList.remove("active"));
    b.classList.add("active");
    action = b.dataset.action;
    $("engine").disabled = !["install", "doctor"].includes(action);
    ["vpn_port", "web_port", "domain", "tls", "restrict_ip"].forEach(id => $(id).disabled = action !== "install");
  });
});
$("auth").addEventListener("change", () => {
  const isPass = $("auth").value === "password";
  $("password").disabled = !isPass;
  $("identity").disabled = isPass;
});
async function api(path, opts) {
  const url = new URL(location.href);
  const r = await fetch(url.origin + path + "?token=" + url.searchParams.get("token"), opts);
  if (!r.ok) throw new Error((await r.text()) || r.status);
  return r.json();
}
$("start").addEventListener("click", async () => {
  $("start").disabled = true;
  $("status").textContent = "Запуск…";
  const body = {
    action: action, host: $("host").value, ssh_port: +$("ssh_port").value || 0,
    user: $("user").value, auth: $("auth").value, identity: $("identity").value,
    known_hosts: $("known_hosts").value, engine: $("engine").value,
    vpn_port: +$("vpn_port").value || 0, web_port: +$("web_port").value || 0,
    domain: $("domain").value, tls: $("tls").value === "true",
    restrict_ip: $("restrict_ip").value, timeout: +$("timeout").value || 0,
    password: $("password").value
  };
  try {
    await api("/api/run", { method: "POST", body: JSON.stringify(body) });
    $("cancel").style.display = "inline-block";
    $("log").textContent = "";
    poll();
  } catch (e) {
    $("status").textContent = "Ошибка запуска";
    $("log").textContent = String(e.message || e);
    $("start").disabled = false;
  }
});
$("cancel").addEventListener("click", async () => {
  try { await api("/api/cancel", { method: "POST" }); } catch (e) {}
});
async function poll() {
  if (timer) clearTimeout(timer);
  try {
    const st = await api("/api/job");
    if (st.output !== undefined) {
      const pre = $("log");
      if (pre.textContent !== st.output) { pre.textContent = st.output || " "; pre.scrollTop = pre.scrollHeight; }
    }
    if (st.running) {
      $("status").textContent = "Выполняется… " + (st.elapsed_s || 0) + " с";
      timer = setTimeout(poll, 900);
    } else if (st.done !== undefined && (st.output !== undefined)) {
      $("status").textContent = st.error ? ("Завершено с ошибкой: " + st.error) : "Готово ✓";
      $("status").className = "status" + (st.error ? " err" : "");
      $("cancel").style.display = "none";
      $("start").disabled = false;
      return;
    }
  } catch (e) {
    $("status").textContent = "Ошибка опроса: " + (e.message || e);
    timer = setTimeout(poll, 2000);
  }
}
</script>
</body>
</html>
`
