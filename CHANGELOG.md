# Changelog

## Unreleased

- Исправлен host-only сценарий: `WG_HOST` берётся из защищённого env-файла и не затирается пустым доменом.
- Домен без `--tls` теперь отклоняется; TLS firewall закрывает прямой backend-порт и оставляет доступ только через proxy или разрешённый IP.
- State записывается после успешного health-check, хранит deployed image и checksum последнего backup; update автоматически восстанавливает snapshot при ошибке.
- Backup расширен до полного deployment snapshot; добавлены fake-SSH lifecycle tests и command-builder tests для P0-контрактов.

## v2.0.0 — 2026-07-25

- Добавлен кроссплатформенный Go CLI `awg-vds` для Windows, Linux и macOS.
- Добавлены команды `install`, `status`, `update`, `backup` и `doctor`.
- Разделены Legacy (WireSock/awg-easy) и экспериментальный Upstream (wg-easy 15.2.1) движки.
- Добавлены remote-first `install-state.json`, атомарная запись состояния и timestamped backup.
- Добавлены SSH-ключи, интерактивная парольная авторизация через системный OpenSSH, preflight и health checks.
- Старый PowerShell установщик сохранён как v1 Legacy.

## v1 Legacy

История изменений Legacy PowerShell-скрипта продолжается в Git history и прежних release notes.
