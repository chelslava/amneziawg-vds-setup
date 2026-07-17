# Быстрая настройка AmneziaWG на VDS

Интерактивный PowerShell 7-скрипт для установки последней версии `ghcr.io/yokitoki/awg-easy` на Ubuntu/Debian.

## Возможности

- Запрашивает IP/DNS VDS, SSH-пользователя и пароль через защищённый ввод Windows.
- Устанавливает Docker, разворачивает AmneziaWG и русскую веб-панель.
- Использует `host`-сеть, исключая известную проблему Docker-proxy с HTTP-сбросами.
- Генерирует отдельный bcrypt-пароль панели и проверяет `healthy`, HTTP и интерфейс VPN.
- Не сохраняет SSH-пароль локально.

## Требования

- PowerShell 7+, Windows OpenSSH Client.
- VDS с Ubuntu/Debian и доступом `root` по SSH с паролем.

## Запуск

```powershell
pwsh -ExecutionPolicy Bypass -File .\Install-AmneziaWG.ps1
```

Или с заранее заданными адресом и портами:

```powershell
pwsh -ExecutionPolicy Bypass -File .\Install-AmneziaWG.ps1 -HostName vpn.example.com -VpnPort 1234 -WebPort 51821
```

После запуска откройте выведенную панель, создайте клиента кнопкой `+` и импортируйте QR-код или `.conf` в AmneziaWG.

## Безопасность

Без домена панель работает по HTTP. Для постоянного доступа настройте TLS reverse proxy. После первоначальной настройки замените пароль `root` на SSH-ключ.

Серверные данные размещаются в `/opt/awg-easy`; не удаляйте эту папку после создания клиентов.
