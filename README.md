# AmneziaWG VDS Setup v2
<img width="836" height="470" alt="amneziawg-vpn-cli-banner-titled836" src="https://github.com/user-attachments/assets/5bea0d29-dd90-4e60-9221-b45e5abcd35f" />

Кроссплатформенный установщик личного AmneziaWG VPN на VDS. Основной язык — Go; один бинарник работает на Windows, Linux и macOS и использует установленный системный OpenSSH. Три интерфейса: интерактивное TUI-меню (Bubble Tea v2), локальная веб-GUI-панель (`awg-vds gui`) и флаговый режим для automation.

## Установка бинарника

Скачайте архив для своей платформы из [GitHub Releases](https://github.com/chelslava/amneziawg-vds-setup/releases), проверьте `SHA256SUMS` и добавьте `awg-vds` в `PATH`.

Для сборки из исходников нужен Go 1.25+:

```powershell
go test ./...
$env:GOOS='windows'; $env:GOARCH='amd64'; go build -trimpath -ldflags='-s -w' -o dist/awg-vds-windows-amd64.exe ./cmd/awg-vds
```

Поддерживаемые артефакты: Windows amd64, Linux amd64, macOS amd64 и arm64. OpenSSH (`ssh`) должен быть доступен в `PATH`. По умолчанию CLI требует уже проверенный ключ сервера в системном `known_hosts` и использует fail-closed host-key checking; для отдельного файла используйте `--known-hosts`. Пароль SSH никогда не принимается флагом: awg-vds запрашивает его один раз без отображения символов, держит только в памяти и передаёт OpenSSH через временный askpass-helper; предпочтителен ключ:

```text
awg-vds install --host vpn.example.com --user root --identity-file ~/.ssh/id_ed25519 --engine legacy
```

## Команды

```text
awg-vds install --host HOST [--ssh-port 22] [--user root] [--identity-file PATH] [--known-hosts PATH]
                 [--engine legacy|upstream] [--vpn-port 1234] [--web-port 51821]
                 [--domain vpn.example.com --tls] [--restrict-panel-ip IP]
awg-vds doctor --host HOST [--engine legacy|upstream]
awg-vds status --host HOST [connection flags]
awg-vds update --host HOST [connection flags]
awg-vds backup --host HOST [connection flags]
awg-vds rotate-password --host HOST [connection flags]
awg-vds gui [--listen 127.0.0.1:8620]
```

### Интерактивное меню

Запуск без аргументов открывает современный keyboard-driven control center на Bubble Tea v2. При первом запуске выбирается English или Русский; выбор сохраняется локально в профиле пользователя (только язык, без серверных секретов) и больше не запрашивается. Сменить язык можно пунктом `Change language` / `Сменить язык`. В форме подключения есть отдельный выбор SSH auth method: `key` или `password`; при `password` пароль вводится следующим prompt’ом самого OpenSSH и не читается приложением. Формы предлагают безопасные значения по умолчанию, позволяют выбрать `legacy`/`upstream`, домен, TLS и IP restriction, а destructive-действия требуют явного ввода `yes`.

Установка в интерактивном режиме показывает отдельный operation screen с этапом, live-логом, таймером `Elapsed`, прокруткой `↑/↓`, отменой через `c`/`Ctrl+C`, сохранением безопасного лога через `s`, адаптивным переносом строк и итоговым статусом. Перед изменяющими операциями отображается summary с host, auth, engine, портами, TLS и timeout.

Каждый этап дополнительно пишет timestamped log `start/completed/failed` с безопасной редацией. На Linux/macOS SSH использует временное multiplexed-соединение. На Windows Win32-OpenSSH не поддерживает `ControlPath`, поэтому используется временный askpass-helper; в обоих случаях пароль запрашивается один раз на запуск операции.

При ошибке TUI показывает безопасную причину и меню восстановления: `Retry`, `Run doctor`, `Use legacy` для применимых upstream-ошибок или `Back`. Возврат в меню выполняется только после явного выбора.

Фиксированные параметры выбираются в выпадающих списках клавишами `↑/↓`, `j/k` и `Enter` (SSH auth, engine, TLS); движок `upstream` предвыбран по умолчанию. `Esc` или ввод `back` возвращает на уровень меню без завершения приложения. Последние профили подключения сохраняются локально в `last-profile.json`; там нет паролей и клиентских конфигураций.

Меню не сохраняет SSH-пароль: при выборе password authentication его один раз запрашивает awg-vds без echo. Для CI используйте `--identity-file`; парольные неинтерактивные запуски намеренно отклоняются. Флаговый режим остаётся доступен для скриптов и CI.

```text
awg-vds
```

### GUI — локальная веб-панель управления

```text
awg-vds gui [--listen 127.0.0.1:8620]
```

Запускает локальный веб-GUI в том же бинарнике и печатает ссылку со случайным
токеном. Сервер слушает только loopback-адреса (0.0.0.0 отклоняется), страница
открывается только по ссылке с токеном. В GUI доступны те же операции, что и в
TUI/CLI: установка, диагностика, статус, обновление, бэкап и смена пароля
панели — с live-журналом операции и кнопкой отмены. Интерфейс на русском.
SSH-пароль вводится в форму, держится только в памяти процесса и не попадает
в журнал операции (ошибки проходят через `ssh.Redact`).

`status`, `update` и `backup` загружают настройки движка, портов, домена и путей из `/opt/awg-vds/install-state.json`. Для SSH всё равно нужны адрес сервера и параметры подключения. `update` сначала создаёт backup и только потом заменяет контейнер. Повторный `install` того же движка выполняет безопасное reconcile; смена движка автоматически запрещена.

`update` создаёт snapshot перед заменой контейнера и автоматически восстанавливает его, если post-update health-check не проходит. Повторный `install` сначала читает state: неизменённая конфигурация безопасно reconcile-ится, а drift портов, домена, TLS или ограничения панели завершается явным diff.

## Движки

### Поддерживаемые серверные системы

Установщик v2 поддерживает Ubuntu, Debian, Fedora, RHEL, CentOS, Rocky Linux,
AlmaLinux и Oracle Linux (`ol`). Для всех вариантов требуется Linux amd64;
Legacy дополнительно сохраняет ограничение amd64 для совместимости с WireSock.
Клиентский бинарник при этом остаётся доступным на Windows, Linux и macOS.

Для Ubuntu/Debian используются подписанные APT-репозитории. Для Fedora/RHEL-
подобных систем используются `dnf`, EPEL при необходимости и Docker Compose.
При upstream без установленного модуля установщик подключает официальный
репозиторий AmneziaWG автоматически (Launchpad PPA на Ubuntu, COPR на
Fedora/RHEL-семействе) — достаточно одной команды install:

```text
dnf copr enable amneziavpn/amneziawg
dnf install amneziawg-dkms amneziawg-tools
```

COPR — сторонний источник пакетов: `doctor` показывает его отсутствие, а
установщик подключает репозиторий автоматически только как подписанный источник
(см. выше). Нужны доступные `kernel-devel`/`kernel-headers`
для запущенного ядра; после обновления ядра перезагрузка выполняется оператором,
не установщиком. Если провайдер использует собственное ядро без headers, выберите
Legacy или другой VDS.

По умолчанию `install` (без `--engine`) и интерактивная форма ставят движок
`upstream` — это последняя версия панели wg-easy 15 с поддержкой AmneziaWG.
Движок `legacy` остаётся доступным явно (`--engine legacy`) для WireSock-
совместимости.

### `legacy` — стабильный сценарий v1

- Образ: `ghcr.io/yokitoki/awg-easy@sha256:92a55ac255302b033967b79e5561a12f6e5a4d8f3442d16631853428c6e65908` (Legacy `0.2.19`, amd64).
- `--network host`, общий каталог `/opt/awg-vds/wireguard` монтируется в `/etc/amnezia/amneziawg` и `/etc/wireguard`.
- Предназначен для Legacy-параметров AmneziaWG и совместимости с WireSock.
- Поддерживается только Linux amd64; ARM отклоняется до начала установки.

### `upstream` — сценарий wg-easy 15 с AmneziaWG

- Образ: `ghcr.io/wg-easy/wg-easy@sha256:071f79a639e9361123a5e57733107320207b844ddbbd92dfa6962f541fe8aac8` (upstream `15.5.0`, поддержка AmneziaWG 3.1).
- Проверяет наличие или installability `amneziawg`, пытается установить пакет из настроенных репозиториев и включает `EXPERIMENTAL_AWG=true`.
- Требует kernel module AmneziaWG; если модуль недоступен, установка останавливается с диагностикой. Поставляемый в образе `amneziawg-go` приложением не используется: интерфейс всегда управляется через `awg-quick` против kernel-модуля.
- Официальные репозитории AmneziaWG ([Launchpad PPA](https://launchpad.net/~amnezia/+archive/ubuntu/ppa) для Ubuntu, COPR для Fedora/RHEL-семейства) подключаются автоматически при отсутствии модуля — и в интерактивном режиме, и в non-interactive «одной командой». Репозитории подписаны, APT/DNF работают с проверкой подписей (`AllowUnauthenticated=false`), поэтому не подписанные метаданные останавливают установку. Debian не поддерживает этот путь и требует ручной настройки.
- Перед upstream-зависимостями выполняется подписанный `apt full-upgrade -y`; если для текущего ядра headers всё ещё недоступны (старые cloud-образы), установщик останавливается с подсказкой: перезагрузите VDS и повторите ту же команду install.
- Пароль панели: wg-easy 15 не принимает пароль через env (`PASSWORD_HASH` роняет контейнер) и хранит argon2-хеши в SQLite-базе. Установщик генерирует пароль в защищённый файл `/opt/awg-vds/panel-password`, после первого старта контейнера сеет администратора (`admin`), VPN-порт и хост панели прямо в базу образа и помечает мастер настройки завершённым. `rotate-password` для upstream перезаписывает файл и пере-хеширует запись в базе.
- Контейнер запускается с монтированием `/lib/modules:ro` (для ip6tables внутри PostUp) и healthcheck на `awg show` вместо штатного `wg show`, который не видит amneziawg-интерфейсы.
- Панель без TLS работает по HTTP: wg-easy 15 блокирует вход по незащищённому соединению, поэтому при `--tls` не выставлен env `INSECURE=true`. Комбинируйте HTTP-панель с `--restrict-panel-ip` — ограничение по IP теперь действует и без TLS (UFW insert allow/deny, firewall-cmd rich rule).
- Это не миграция Legacy и не обновление Legacy. Разворачивайте его как отдельную новую установку.
- Upstream AmneziaWG остаётся экспериментальным; штатный healthcheck образа в AWG-режиме всегда unhealthy, поэтому v2 переопределяет его и дополнительно проверяет контейнер, локальный HTTP, `awg/wg show` и TCP/UDP listeners.

Legacy и AmneziaWG 2.0 — разные сценарии. v2 не генерирует CPS `I1–I5` автоматически и не считает, что все AWG-параметры обязаны совпадать: у `Jc/Jmin/Jmax`, `S1–S4`, `H1–H4` и `I1–I5` разные правила совместимости. CPS можно будет добавить отдельным явно документированным режимом.
Будущий контракт opt-in и parser fixtures описаны в [`docs/cps-contract.md`](docs/cps-contract.md); до его реализации CPS остаётся `disabled`.

## Doctor и проверки

`doctor` проверяет SSH, дистрибутив и версию, kernel/архитектуру, свободное место и память, Docker/Compose, занятость TCP/UDP портов, UFW/firewalld/nftables, DNS для домена и поддержку AmneziaWG для upstream. После установки выполняются проверки контейнера, локальной панели, UDP VPN-порта, TCP-панели и `awg show`/`wg show`.

Для `install` стандартный timeout SSH — 30 минут, чтобы `apt full-upgrade`, `dnf install` и DKMS успевали завершиться на малых VDS. Для особенно медленного VDS можно указать, например, `--timeout 2400`; автоматическая перезагрузка сервера не выполняется.

## State и backup

`--domain` используется только вместе с `--tls`; домен без TLS CLI отклоняет. После успешного health-check state получает deployed image, panel host и метаданные последнего backup. Финальный summary отдельно показывает remote-local health и операторскую external panel reachability.

Серверное состояние хранится в `/opt/awg-vds/install-state.json` и содержит engine, pinned image, порты, домен, TLS-режим, даты, пути и метаданные последнего backup (путь и SHA-256). SSH-пароли, приватные ключи, `PASSWORD_HASH` и клиентские конфигурации туда не записываются.

При установке панель получает случайный пароль: исходный пароль хранится только на VDS в /opt/awg-vds/panel-password с 0600 и не попадает в state, operation logs или флаговый CLI-вывод. Legacy использует bcrypt-хеш через env; upstream (wg-easy 15) хранит argon2-хеш в своей базе — установщик сеет запись администратора `admin` после первого старта контейнера. После успешного интерактивного `install` финальное TUI-окно показывает сохраняемую карточку доступа: URL панели, логин `admin` для upstream, пароль панели, VPN UDP-порт, engine, SSH-команду и команду повторного чтения пароля; сохраните её в менеджере паролей. Повторно получить его можно отдельной SSH-командой `ssh root@HOST sudo cat /opt/awg-vds/panel-password`.

Для плановой смены или восстановления доступа используйте `rotate-password`. Команда сначала создаёт backup, атомарно меняет защищённый файл пароля, проверяет health и удаляет временную копию старого секрета только после успеха. Механика зависит от движка: legacy получает новый bcrypt-хеш через env-файл, upstream — новый argon2-хеш записи администратора в базе панели (контейнер на время пересева останавливается). Новый пароль не выводится флаговым CLI, GUI или operation logs; получите его отдельной интерактивной SSH-командой `ssh root@HOST sudo cat /opt/awg-vds/panel-password`. При сбое health выполняется rollback к старому паролю; plaintext не попадает в state, логи или GitHub Actions.

Backup создаётся на сервере в `/opt/awg-vds/backups/` с UTC-датой в имени, SHA-256 и правами `0600`. Snapshot включает VPN-конфигурацию, env-файлы, state, Caddy-данные и защищённый файл пароля панели, поэтому не копируйте его в публичное место. `update` не продолжает обновление, если backup не создан; при последующем health failure snapshot используется для rollback.

Без `--tls` панель остаётся доступной по HTTP. CLI выводит предупреждение: используйте домен с Caddy TLS и/или ограничение панели по IP через `--restrict-panel-ip`; также откройте только необходимые порты в firewall/security group.

## Безопасность

- Не используйте `--password`, `--ssh-password` и другие секретные аргументы — CLI их отклоняет.
- Не передавайте `PASSWORD_HASH`, приватные ключи или клиентские `.conf` через логи.
- Образы v2 используют immutable digest; обновление происходит только для явно закреплённого image reference в state. Legacy v1 PowerShell сохраняет отдельный исторический `latest` контракт.
- APT используется только через системные подписанные метаданные Ubuntu/Debian: v2 передаёт `AllowInsecureRepositories=false` и `AllowUnauthenticated=false`, поэтому unsigned/unauthenticated source останавливает установку. Официальные репозитории AmneziaWG (Launchpad PPA и COPR) подключаются автоматически и только как подписанные источники; не подписанные метаданные всё равно прерывают установку. Обновление digest или action SHA требует сверки upstream release, локального `go test`/`go vet` и CI run; исходные v1 pins не переносите в v2 автоматически.
- Передавайте ключ через `--identity-file`; после настройки отключите SSH password login и root password login.
- Проверьте ownership VDS до запуска. TLS требует, чтобы DNS уже указывал на VDS.

## v1 Legacy

`Install-AmneziaWG.ps1` намеренно не удалён. Это Legacy v1 для существующих пользователей: он требует PowerShell 7, сохраняет прежний сценарий awg-easy/WireSock, TLS/UFW и совместимость с `/opt/awg-easy`. Не смешивайте каталоги v1 и v2 и не запускайте v2 с движком upstream поверх v1-сервера.

Запуск v1:

```powershell
pwsh -ExecutionPolicy Bypass -File .\Install-AmneziaWG.ps1
```

## Разработка и релиз

```powershell
go test ./...
go vet ./...
$env:GOOS='windows'; $env:GOARCH='amd64'; go build -trimpath -ldflags='-s -w' -o dist/awg-vds-windows-amd64.exe ./cmd/awg-vds
$env:GOOS='linux'; $env:GOARCH='amd64'; go build -trimpath -ldflags='-s -w' -o dist/awg-vds-linux-amd64 ./cmd/awg-vds
$env:GOOS='darwin'; $env:GOARCH='amd64'; go build -trimpath -ldflags='-s -w' -o dist/awg-vds-darwin-amd64 ./cmd/awg-vds
$env:GOOS='darwin'; $env:GOARCH='arm64'; go build -trimpath -ldflags='-s -w' -o dist/awg-vds-darwin-arm64 ./cmd/awg-vds
```

Workflow `.github/workflows/release.yml` повторяет тесты, собирает четыре бинарника (Windows/Linux amd64, macOS amd64/arm64 — все с GUI и TUI) и публикует `SHA256SUMS` для version tag.
