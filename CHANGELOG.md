# Changelog

## Unreleased

- Расширена серверная поддержка на Fedora, RHEL/CentOS, Rocky Linux, AlmaLinux
  и Oracle Linux: добавлены dnf/DKMS-зависимости, диагностика репозиториев и
  firewalld. Для upstream подключение официального AmneziaWG COPR требует
  явного подтверждения в интерактивном режиме.
- Исправлена установка зависимостей на Debian: Compose больше не считается
  достаточным признаком Docker; CLI выбирается из доступных `docker.io`,
  `docker-ce-cli` или `docker-cli` и проверяется до запуска engine.
- Проверка доступности `linux-headers` теперь использует реальный APT Candidate,
  поэтому установщик не пытается установить пакет, для которого репозиторий
  предоставляет только метаданные без версии-кандидата.

- TUI получил operation screen с live-логами, elapsed timer, scroll/cancel,
  summary перед изменениями, recovery actions после ошибок и безопасные локальные
  профили подключения без секретов.

- Ошибка отсутствующего upstream-модуля AmneziaWG теперь показывает отдельные
  рекомендации: проверить kernel/module через `doctor` или выбрать `legacy` для
  VDS без поддержки upstream.
- Интерактивная установка upstream на Ubuntu теперь после такой ошибки спрашивает
  разрешение добавить официальный AmneziaWG PPA и повторяет preflight; без явного
  подтверждения APT-источники не меняются.
- Исправлена установка kernel module после PPA: перед `amneziawg` добавлены
  текущие kernel headers, DKMS, build tools и Launchpad prerequisites; ошибка
  `modprobe` получает отдельные рекомендации вместо SSH-подсказки.
- Перед всеми upstream-зависимостями выполняется `apt full-upgrade -y`; reboot
  никогда не выполняется автоматически, а требуется только отдельной рекомендацией.
- Для `install` стандартный SSH timeout увеличен до 15 минут, чтобы полный upgrade
  и DKMS-сборка не обрывались через 120 секунд; `--timeout` остаётся доступен.

- Исправлен Windows-сбой `getsockname failed: Not a socket`: `ControlPath` теперь
  используется только на Unix, а пароль вводится один раз без echo через
  временный askpass-helper, только в памяти текущей операции.
- Добавлен regression-тест, запрещающий Unix control socket на Windows.

## v2.2.4 — 2026-07-25

- SSH-команды установки используют временный ControlMaster/ControlPath, поэтому пароль запрашивается один раз за операцию.
- Прогресс установки дополнен timestamped логами начала, завершения и ошибки каждого этапа.

## v2.2.3 — 2026-07-25

- Добавлены выпадающие списки для SSH auth, engine и TLS с клавиатурной навигацией.
- `Esc` и `back` возвращают из формы на уровень меню; выборы больше не вводятся вручную.
- Line-oriented тестовый/automation helper отделён от Bubble Tea ввода.

## v2.2.2 — 2026-07-25

- Ошибки интерактивных операций теперь показываются на отдельном экране с безопасной причиной и рекомендациями по устранению.
- Для запрета Legacy → Upstream добавлена явная подсказка использовать чистый VDS или повторить установку с тем же движком.

## v2.2.1 — 2026-07-25

- Исправлен первый запуск TUI: выбор языка завершается отдельно, после чего открывается главное меню.
- После каждой операции интерфейс явно возвращается в главное меню; добавлен пошаговый прогресс установки.
- Локальное предпочтение языка сохраняется один раз и может быть изменено из меню.

## v2.2.0 — 2026-07-25

- Интерактивное меню переведено на Bubble Tea v2 с alternate screen, клавиатурной навигацией и первым выбором языка `English` / `Русский`.
- Выбор языка сохраняется локально один раз; в главном меню добавлен пункт смены языка.
- Установка в TUI показывает пошаговый прогресс preflight, зависимостей, запуска, сетевой настройки, health-check и сохранения state.

## v2.1.0 — 2026-07-25

- Добавлен кроссплатформенный интерактивный control center без внешних TUI-зависимостей: меню операций, формы подключения, выбор engine/TLS, defaults и подтверждения destructive-действий.

## v2.0.1 — 2026-07-25

- Исправлен host-only сценарий: `WG_HOST` берётся из защищённого env-файла и не затирается пустым доменом.
- Домен без `--tls` теперь отклоняется; TLS firewall закрывает прямой backend-порт и оставляет доступ только через proxy или разрешённый IP.
- State записывается после успешного health-check, хранит deployed image и checksum последнего backup; update автоматически восстанавливает snapshot при ошибке.
- Backup расширен до полного deployment snapshot; добавлены fake-SSH lifecycle tests и command-builder tests для P0-контрактов.
- Повторный `install` теперь читает state до проверки занятых портов, принимает ожидаемый busy порт при безопасном reconcile и явно сообщает configuration drift вместо его молчаливого игнорирования.
- SSH теперь использует fail-closed host-key verification (`StrictHostKeyChecking=yes`) с опциональным `--known-hosts`; remote stderr проходит тот же secret redaction, что и stdout.
- Production v2 images переведены на immutable digests; Legacy использует существующий GHCR `0.2.15`, поскольку прежний tag `1.0.1` больше не опубликован. Legacy v1 `latest` не изменён.
- State validation теперь ограничивает engine-specific container/image contracts, managed paths под `/opt/awg-vds`, panel IP и backup metadata; tampered lifecycle state отклоняется до выполнения remote command.
- Doctor обновляет APT metadata перед upstream module probe, dependencies гарантируют `curl`, а package install и `modprobe` failures получают явные diagnostics.
- Summary теперь показывает фактический panel URL с web port для HTTP и `https://domain` для TLS, а также отдельный внешний reachability probe.
- Engine factory теперь использует canonical `internal/engine/legacy` и `internal/engine/upstream`; divergent `compat.go` удалён, включая старый `WG_HOST` и DNS contract.
- Добавлен `rotate-password` с атомарной сменой credential, health-check и rollback без вывода plaintext.
- CI отделён от release: Go test/vet, native Windows/Linux/macOS smoke, immutable-reference scan и pinned Pester 5.7.1 для v1 Legacy.
- Добавлен manual disposable-VDS E2E harness для Ubuntu/Debian с pinned known_hosts и always-cleanup; provider provisioning остаётся настройкой GitHub environment.
- APT dependency paths теперь fail-closed при insecure или unauthenticated repositories.
- Описан design-only CPS I1–I5 контракт с явным opt-in, раздельными правилами matching, secret boundaries и parser fixtures; автоматическая генерация не включена.

## v2.0.0 — 2026-07-25

- Добавлен кроссплатформенный Go CLI `awg-vds` для Windows, Linux и macOS.
- Добавлены команды `install`, `status`, `update`, `backup` и `doctor`.
- Разделены Legacy (WireSock/awg-easy) и экспериментальный Upstream (wg-easy 15.2.1) движки.
- Добавлены remote-first `install-state.json`, атомарная запись состояния и timestamped backup.
- Добавлены SSH-ключи, интерактивная парольная авторизация через системный OpenSSH, preflight и health checks.
- Старый PowerShell установщик сохранён как v1 Legacy.

## v1 Legacy

История изменений Legacy PowerShell-скрипта продолжается в Git history и прежних release notes.
