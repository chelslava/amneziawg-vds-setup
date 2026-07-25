# Changelog

## v2.0.0 — 2026-07-25

- Добавлен кроссплатформенный Go CLI `awg-vds` для Windows, Linux и macOS.
- Добавлены команды `install`, `status`, `update`, `backup` и `doctor`.
- Разделены Legacy (WireSock/awg-easy) и экспериментальный Upstream (wg-easy 15.2.1) движки.
- Добавлены remote-first `install-state.json`, атомарная запись состояния и timestamped backup.
- Добавлены SSH-ключи, интерактивная парольная авторизация через системный OpenSSH, preflight и health checks.
- Старый PowerShell установщик сохранён как v1 Legacy.

## v1 Legacy

История изменений Legacy PowerShell-скрипта продолжается в Git history и прежних release notes.
