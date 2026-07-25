# awg-vds v2

Кроссплатформенный консольный установщик личного AmneziaWG VPN на VDS. Основной язык — Go; один бинарник работает на Windows, Linux и macOS и использует установленный системный OpenSSH. Интерактивный интерфейс построен на Bubble Tea v2; флаговый режим сохраняется для automation.

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
```

### Интерактивное меню

Запуск без аргументов открывает современный keyboard-driven control center на Bubble Tea v2. При первом запуске выбирается English или Русский; выбор сохраняется локально в профиле пользователя (только язык, без серверных секретов) и больше не запрашивается. Сменить язык можно пунктом `Change language` / `Сменить язык`. В форме подключения есть отдельный выбор SSH auth method: `key` или `password`; при `password` пароль вводится следующим prompt’ом самого OpenSSH и не читается приложением. Формы предлагают безопасные значения по умолчанию, позволяют выбрать `legacy`/`upstream`, домен, TLS и IP restriction, а destructive-действия требуют явного ввода `yes`.

Установка в интерактивном режиме показывает отдельный operation screen с этапом, live-логом, таймером `Elapsed`, прокруткой `↑/↓`, отменой через `c`/`Ctrl+C`, сохранением безопасного лога через `s`, адаптивным переносом строк и итоговым статусом. Перед изменяющими операциями отображается summary с host, auth, engine, портами, TLS и timeout.

Каждый этап дополнительно пишет timestamped log `start/completed/failed` с безопасной редацией. На Linux/macOS SSH использует временное multiplexed-соединение. На Windows Win32-OpenSSH не поддерживает `ControlPath`, поэтому используется временный askpass-helper; в обоих случаях пароль запрашивается один раз на запуск операции.

При ошибке TUI показывает безопасную причину и меню восстановления: `Retry`, `Run doctor`, `Use legacy` для применимых upstream-ошибок или `Back`. Возврат в меню выполняется только после явного выбора.

Фиксированные параметры выбираются в выпадающих списках клавишами `↑/↓`, `j/k` и `Enter` (SSH auth, engine, TLS). `Esc` или ввод `back` возвращает на уровень меню без завершения приложения. Последние профили подключения сохраняются локально в `last-profile.json`; там нет паролей и клиентских конфигураций.

```text
awg-vds
```

Меню не сохраняет SSH-пароль: при выборе password authentication его один раз запрашивает awg-vds без echo. Для CI используйте `--identity-file`; парольные неинтерактивные запуски намеренно отклоняются. Флаговый режим остаётся доступен для скриптов и CI.

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
При upstream без установленного модуля установщик только после явного согласия
может подключить официальный AmneziaWG COPR:

```text
dnf copr enable amneziavpn/amneziawg
dnf install amneziawg-dkms amneziawg-tools
```

COPR является сторонним источником пакетов: `doctor` показывает его отсутствие,
а интерактивный `install` предлагает подключение. Non-interactive режим не
меняет репозитории автоматически. Нужны доступные `kernel-devel`/`kernel-headers`
для запущенного ядра; после обновления ядра перезагрузка выполняется оператором,
не установщиком. Если провайдер использует собственное ядро без headers, выберите
Legacy или другой VDS.

### `legacy` — стабильный сценарий v1

- Образ: `ghcr.io/yokitoki/awg-easy@sha256:bfb9070d88379dc31ce55ef5588915964a2c3abd657249c696dd375202df3f6f` (Legacy `0.2.15`, amd64).
- `--network host`, общий каталог `/opt/awg-vds/wireguard` монтируется в `/etc/amnezia/amneziawg` и `/etc/wireguard`.
- Предназначен для Legacy-параметров AmneziaWG и совместимости с WireSock.
- Поддерживается только Linux amd64; ARM отклоняется до начала установки.

### `upstream` — экспериментальный сценарий

- Образ: `ghcr.io/wg-easy/wg-easy@sha256:4ffc03c35dce5456bbb2fa6b136a1eeb196394548dee0650ae692efdd1062e01` (upstream `15.2.1`).
- Проверяет наличие или installability `amneziawg`, пытается установить пакет из настроенных репозиториев и включает `EXPERIMENTAL_AWG=true`.
- Требует kernel module AmneziaWG; если модуль недоступен, установка останавливается с диагностикой.
- В интерактивном `install` на Ubuntu при отсутствии пакета предлагает явно добавить официальный [AmneziaWG PPA](https://launchpad.net/~amnezia/+archive/ubuntu/ppa), затем повторяет preflight. Ответ `no`, non-interactive CLI и Debian не изменяют APT-источники автоматически.
- Перед upstream-зависимостями выполняется подписанный `apt full-upgrade -y`; если для текущего ядра headers всё ещё недоступны, установщик останавливается и просит вручную перезагрузить VDS после обновления ядра.
- Это не миграция Legacy и не обновление Legacy. Разворачивайте его как отдельную новую установку.
- Upstream AmneziaWG остаётся экспериментальным; upstream предупреждает, что AmneziaWG может ломать стандартный healthcheck. Поэтому v2 дополнительно проверяет контейнер, локальный HTTP, `awg/wg show` и TCP/UDP listeners.

Legacy и AmneziaWG 2.0 — разные сценарии. v2 не генерирует CPS `I1–I5` автоматически и не считает, что все AWG-параметры обязаны совпадать: у `Jc/Jmin/Jmax`, `S1–S4`, `H1–H4` и `I1–I5` разные правила совместимости. CPS можно будет добавить отдельным явно документированным режимом.
Будущий контракт opt-in и parser fixtures описаны в [`docs/cps-contract.md`](docs/cps-contract.md); до его реализации CPS остаётся `disabled`.

## Doctor и проверки

`doctor` проверяет SSH, дистрибутив и версию, kernel/архитектуру, свободное место и память, Docker/Compose, занятость TCP/UDP портов, UFW/firewalld/nftables, DNS для домена и поддержку AmneziaWG для upstream. После установки выполняются проверки контейнера, локальной панели, UDP VPN-порта, TCP-панели и `awg show`/`wg show`.

Для `install` стандартный timeout SSH — 15 минут, чтобы `apt full-upgrade` и DKMS успевали завершиться. Для особенно медленного VDS можно указать, например, `--timeout 1800`; автоматическая перезагрузка сервера не выполняется.

## State и backup

`--domain` используется только вместе с `--tls`; домен без TLS CLI отклоняет. После успешного health-check state получает deployed image, panel host и метаданные последнего backup. Финальный summary отдельно показывает remote-local health и операторскую external panel reachability.

Серверное состояние хранится в `/opt/awg-vds/install-state.json` и содержит engine, pinned image, порты, домен, TLS-режим, даты, пути и метаданные последнего backup (путь и SHA-256). SSH-пароли, приватные ключи, `PASSWORD_HASH` и клиентские конфигурации туда не записываются.

При установке панель получает случайный bcrypt-пароль: исходный пароль хранится только на VDS в /opt/awg-vds/panel-password с 0600 и не попадает в state, логи или вывод CLI. Получайте его осознанно отдельной SSH-командой ssh root@HOST sudo cat /opt/awg-vds/panel-password и не сохраняйте в публичных местах.

Для плановой смены или восстановления доступа используйте `rotate-password`. Команда сначала создаёт backup, атомарно меняет защищённый файл и `PASSWORD_HASH`, перезапускает только контейнер панели, проверяет health и удаляет временную копию старого секрета только после успеха. Новый пароль не выводится CLI: получите его отдельной интерактивной командой `ssh root@HOST sudo cat /opt/awg-vds/panel-password`. При сбое health выполняется rollback; plaintext не попадает в state, логи или GitHub Actions.

Backup создаётся на сервере в `/opt/awg-vds/backups/` с UTC-датой в имени, SHA-256 и правами `0600`. Snapshot включает VPN-конфигурацию, env-файлы, state, Caddy-данные и защищённый файл пароля панели, поэтому не копируйте его в публичное место. `update` не продолжает обновление, если backup не создан; при последующем health failure snapshot используется для rollback.

Без `--tls` панель остаётся доступной по HTTP. CLI выводит предупреждение: используйте домен с Caddy TLS и/или ограничение панели по IP через `--restrict-panel-ip`; также откройте только необходимые порты в firewall/security group.

## Безопасность

- Не используйте `--password`, `--ssh-password` и другие секретные аргументы — CLI их отклоняет.
- Не передавайте `PASSWORD_HASH`, приватные ключи или клиентские `.conf` через логи.
- Образы v2 используют immutable digest; обновление происходит только для явно закреплённого image reference в state. Legacy v1 PowerShell сохраняет отдельный исторический `latest` контракт.
- APT используется только через системные подписанные метаданные Ubuntu/Debian: v2 передаёт `AllowInsecureRepositories=false` и `AllowUnauthenticated=false`, поэтому unsigned/unauthenticated source останавливает установку. Для Fedora/RHEL COPR подключается только после явного подтверждения пользователя. Обновление digest или action SHA требует сверки upstream release, локального `go test`/`go vet` и CI run; исходные v1 pins не переносите в v2 автоматически.
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
$env:GOOS='windows'; $env:GOARCH='amd64'; go build -trimpath -o dist/awg-vds-windows-amd64.exe ./cmd/awg-vds
$env:GOOS='linux'; $env:GOARCH='amd64'; go build -trimpath -o dist/awg-vds-linux-amd64 ./cmd/awg-vds
```

Workflow `.github/workflows/release.yml` повторяет тесты, собирает четыре бинарника и публикует `SHA256SUMS` для version tag.
