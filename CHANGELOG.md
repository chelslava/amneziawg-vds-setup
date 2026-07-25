# Changelog

## Unreleased

- Исправлен host-only сценарий: `WG_HOST` берётся из защищённого env-файла и не затирается пустым доменом.
- Домен без `--tls` теперь отклоняется; TLS firewall закрывает прямой backend-порт и оставляет доступ только через proxy или разрешённый IP.
- State записывается после успешного health-check, хранит deployed image и checksum последнего backup; update автоматически восстанавливает snapshot при ошибке.
- Backup расширен до полного deployment snapshot; добавлены fake-SSH lifecycle tests и command-builder tests для P0-контрактов.
- Повторный `install` теперь читает state до проверки занятых портов, принимает ожидаемый busy порт при безопасном reconcile и явно сообщает configuration drift вместо его молчаливого игнорирования.
- SSH теперь использует fail-closed host-key verification (`StrictHostKeyChecking=yes`) с опциональным `--known-hosts`; remote stderr проходит тот же secret redaction, что и stdout.
- Production v2 images переведены на immutable digests; Legacy использует существующий GHCR `0.2.15`, поскольку прежний tag `1.0.1` больше не опубликован. Legacy v1 `latest` не изменён.
- State validation теперь ограничивает engine-specific container/image contracts, managed paths под `/opt/awg-vds`, panel IP и backup metadata; tampered lifecycle state отклоняется до выполнения remote command.
- Doctor обновляет APT metadata перед upstream module probe, dependencies гарантируют `curl`, а package install и `modprobe` failures получают явные diagnostics.

## v2.0.0 — 2026-07-25

- Добавлен кроссплатформенный Go CLI `awg-vds` для Windows, Linux и macOS.
- Добавлены команды `install`, `status`, `update`, `backup` и `doctor`.
- Разделены Legacy (WireSock/awg-easy) и экспериментальный Upstream (wg-easy 15.2.1) движки.
- Добавлены remote-first `install-state.json`, атомарная запись состояния и timestamped backup.
- Добавлены SSH-ключи, интерактивная парольная авторизация через системный OpenSSH, preflight и health checks.
- Старый PowerShell установщик сохранён как v1 Legacy.

## v1 Legacy

История изменений Legacy PowerShell-скрипта продолжается в Git history и прежних release notes.
