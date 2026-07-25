# awg-vds v2

Кроссплатформенный консольный установщик личного AmneziaWG VPN на VDS. Основной язык — Go; один бинарник работает на Windows, Linux и macOS и использует установленный системный OpenSSH.

## Установка бинарника

Скачайте архив для своей платформы из [GitHub Release v2.0.0](https://github.com/chelslava/amneziawg-vds-setup/releases/tag/v2.0.0), проверьте `SHA256SUMS` и добавьте `awg-vds` в `PATH`.

Для сборки из исходников:

```powershell
go test ./...
$env:GOOS='windows'; $env:GOARCH='amd64'; go build -trimpath -ldflags='-s -w' -o dist/awg-vds-windows-amd64.exe ./cmd/awg-vds
```

Поддерживаемые артефакты: Windows amd64, Linux amd64, macOS amd64 и arm64. OpenSSH (`ssh`) должен быть доступен в `PATH`. По умолчанию CLI требует уже проверенный ключ сервера в системном `known_hosts` и использует fail-closed host-key checking; для отдельного файла используйте `--known-hosts`. Пароль SSH никогда не принимается флагом и вводится только самим OpenSSH в интерактивном режиме; предпочтителен ключ:

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

Запуск без аргументов открывает line-oriented control center с меню Install, Status, Doctor, Update, Backup и Rotate password. Формы предлагают безопасные значения по умолчанию, позволяют выбрать `legacy`/`upstream`, домен, TLS и IP restriction, а destructive-действия требуют явного ввода `yes`.

```text
awg-vds
```

Меню не запрашивает и не сохраняет SSH-пароль: при выборе password authentication его запрашивает системный OpenSSH. Флаговый режим остаётся доступен для скриптов и CI.

`status`, `update` и `backup` загружают настройки движка, портов, домена и путей из `/opt/awg-vds/install-state.json`. Для SSH всё равно нужны адрес сервера и параметры подключения. `update` сначала создаёт backup и только потом заменяет контейнер. Повторный `install` того же движка выполняет безопасное reconcile; смена движка автоматически запрещена.

`update` создаёт snapshot перед заменой контейнера и автоматически восстанавливает его, если post-update health-check не проходит. Повторный `install` сначала читает state: неизменённая конфигурация безопасно reconcile-ится, а drift портов, домена, TLS или ограничения панели завершается явным diff.

## Движки

### `legacy` — стабильный сценарий v1

- Образ: `ghcr.io/yokitoki/awg-easy@sha256:bfb9070d88379dc31ce55ef5588915964a2c3abd657249c696dd375202df3f6f` (Legacy `0.2.15`, amd64).
- `--network host`, общий каталог `/opt/awg-vds/wireguard` монтируется в `/etc/amnezia/amneziawg` и `/etc/wireguard`.
- Предназначен для Legacy-параметров AmneziaWG и совместимости с WireSock.
- Поддерживается только Linux amd64; ARM отклоняется до начала установки.

### `upstream` — экспериментальный сценарий

- Образ: `ghcr.io/wg-easy/wg-easy@sha256:4ffc03c35dce5456bbb2fa6b136a1eeb196394548dee0650ae692efdd1062e01` (upstream `15.2.1`).
- Проверяет наличие или installability `amneziawg`, пытается установить пакет из настроенных репозиториев и включает `EXPERIMENTAL_AWG=true`.
- Требует kernel module AmneziaWG; если модуль недоступен, установка останавливается с диагностикой.
- Это не миграция Legacy и не обновление Legacy. Разворачивайте его как отдельную новую установку.
- Upstream AmneziaWG остаётся экспериментальным; upstream предупреждает, что AmneziaWG может ломать стандартный healthcheck. Поэтому v2 дополнительно проверяет контейнер, локальный HTTP, `awg/wg show` и TCP/UDP listeners.

Legacy и AmneziaWG 2.0 — разные сценарии. v2 не генерирует CPS `I1–I5` автоматически и не считает, что все AWG-параметры обязаны совпадать: у `Jc/Jmin/Jmax`, `S1–S4`, `H1–H4` и `I1–I5` разные правила совместимости. CPS можно будет добавить отдельным явно документированным режимом.
Будущий контракт opt-in и parser fixtures описаны в [`docs/cps-contract.md`](docs/cps-contract.md); до его реализации CPS остаётся `disabled`.

## Doctor и проверки

`doctor` проверяет SSH, Ubuntu/Debian, kernel/архитектуру, свободное место и память, Docker/Compose, занятость TCP/UDP портов, firewall, DNS для домена и поддержку AmneziaWG для upstream. После установки выполняются проверки контейнера, локальной панели, UDP VPN-порта, TCP-панели и `awg show`/`wg show`.

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
- APT используется только через системные подписанные метаданные Ubuntu/Debian: v2 передаёт `AllowInsecureRepositories=false` и `AllowUnauthenticated=false`, поэтому unsigned/unauthenticated source останавливает установку. Обновление digest или action SHA требует сверки upstream release, локального `go test`/`go vet` и CI run; исходные v1 pins не переносите в v2 автоматически.
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
