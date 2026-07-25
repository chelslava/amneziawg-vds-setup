# Инструкция для агентов

## Назначение
Текущий продукт v2 — `awg-vds`, кроссплатформенный Go CLI для развёртывания AmneziaWG по SSH. `Install-AmneziaWG.ps1` сохранён как v1 Legacy и не должен задавать архитектуру v2.

## Безопасный порядок работы
1. Для v2 используй адрес VDS, SSH-пользователя и ключ; пароль допустим только интерактивно через OpenSSH.
2. Не записывай SSH-пароль, ключи, `PASSWORD_HASH`, конфигурации клиентов или пароль веб-панели в репозиторий, логи либо документацию.
3. Перед запуском проверь, что VDS принадлежит пользователю и SSH доступен.
4. Для v2 проверяй `go test ./...`, `go vet ./...` и четыре cross-build артефакта; v1 запускай из PowerShell 7.
5. После запуска проверь `healthy`, HTTP 200 панели, UDP VPN-порт и `awg show`.
6. Сообщай пользователю URL панели и пароль только в защищённом ответе; рекомендуй сменить root-пароль на SSH-ключ и настроить TLS.

## v2 технические инварианты
- State: `/opt/awg-vds/install-state.json`; backups: `/opt/awg-vds/backups/`.
- Образы v2 задаются immutable digest; `latest` запрещён CI-проверкой.
- `install` не мигрирует Legacy ↔ Upstream; повторный запуск того же engine делает reconcile и отклоняет drift.
- Update сначала создаёт полный backup и выполняет rollback после неуспешного health-check.
- SSH требует `StrictHostKeyChecking=yes`; секреты, клиентские конфиги и `PASSWORD_HASH` не выводятся.
- По умолчанию: VPN UDP `1234`, панель TCP `51821`; TLS или IP restriction обязателен для безопасной внешней панели.

## v1 Legacy инварианты
- Образ сохраняется как `ghcr.io/yokitoki/awg-easy:latest` для совместимости пользователей v1; не переносить этот tag в v2.
- Контейнер использует `--network host`, а каталог `/opt/awg-easy/wireguard` монтируется в `/etc/wireguard` и `/etc/amnezia/amneziawg`.
- sysctl хоста: `net.ipv4.ip_forward=1`, `net.ipv4.conf.all.src_valid_mark=1`.
- Legacy выдаёт WireSock/Legacy-совместимые параметры, не AmneziaWG 2.0.

## Checklist для изменений обеих генераций
- Определи, затрагивает ли изменение v2, v1 или оба поколения.
- Для v2 обнови Go-тесты, README/CHANGELOG и CI; не добавляй floating tags или секреты в state.
- Для v1 сохрани host networking, оба mount path и интерактивную credential модель; проверь Pester contract.
- Не делай автоматическую миграцию между engines и не меняй v1 поведение ради v2.

## Ограничения
Панель выдаёт AmneziaWG Legacy-совместимые конфигурации, а не официальный AmneziaWG 2.0. Не добавляй persistence секретов ради автоматизации.
