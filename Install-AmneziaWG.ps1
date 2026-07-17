#requires -Version 7.0
[CmdletBinding()]
param(
    [string]$HostName,
    [string]$SshUser = 'root',
    [int]$VpnPort = 1234,
    [int]$WebPort = 51821
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

function Require-Value([string]$Name, [string]$Value) {
    if ([string]::IsNullOrWhiteSpace($Value)) { throw "$Name is required." }
}

if (-not $HostName) { $HostName = Read-Host 'IP-адрес или DNS-имя VDS' }
Require-Value 'HostName' $HostName
if ($HostName -notmatch '^[a-zA-Z0-9.-]+$') { throw 'Недопустимое имя хоста.' }
if ($SshUser -notmatch '^[a-z_][a-z0-9_-]*$') { throw 'Недопустимое имя SSH-пользователя.' }
if ($VpnPort -notin 1..65535 -or $WebPort -notin 1..65535 -or $VpnPort -eq $WebPort) { throw 'Проверьте порты.' }
if (-not (Get-Command ssh -ErrorAction SilentlyContinue)) { throw 'Не найден OpenSSH Client.' }

$credential = Get-Credential -UserName $SshUser -Message "SSH-пароль для $SshUser@$HostName"
$password = $credential.GetNetworkCredential().Password
Require-Value 'SSH password' $password
$askPass = Join-Path $env:TEMP "awg-askpass-$([guid]::NewGuid()).cmd"

try {
    Set-Content -LiteralPath $askPass -Value @('@echo off', 'echo %AWG_SSH_PASSWORD%') -Encoding ascii
    $env:AWG_SSH_PASSWORD = $password
    $env:SSH_ASKPASS = $askPass
    $env:SSH_ASKPASS_REQUIRE = 'force'
    $env:DISPLAY = '1'

    $remote = @'
set -euo pipefail
export DEBIAN_FRONTEND=noninteractive
. /etc/os-release
case "$ID" in ubuntu|debian) ;; *) echo "Unsupported OS: $ID" >&2; exit 1;; esac
apt-get update
apt-get install -y docker.io apache2-utils openssl
systemctl enable --now docker
cat >/etc/sysctl.d/99-amneziawg.conf <<EOF
net.ipv4.ip_forward=1
net.ipv4.conf.all.src_valid_mark=1
EOF
sysctl --system >/dev/null
install -d -m 700 /opt/awg-easy/wireguard
WEB_PASSWORD="$(openssl rand -hex 16)"
WEB_HASH="$(htpasswd -nbBC 12 '' "$WEB_PASSWORD" | cut -d: -f2)"
WG_DEVICE="$(ip route show default | awk '{print $5; exit}')"
cat >/opt/awg-easy/.env <<EOF
WG_HOST=__HOST__
WG_PORT=__VPN_PORT__
WEB_PORT=__WEB_PORT__
WG_DEVICE=${WG_DEVICE}
PASSWORD_HASH=${WEB_HASH}
EOF
chmod 600 /opt/awg-easy/.env
docker pull ghcr.io/yokitoki/awg-easy:latest
docker rm -f amnezia-wg-easy 2>/dev/null || true
docker run -d --name amnezia-wg-easy --network host --env-file /opt/awg-easy/.env -e LANG=ru -e UI_TRAFFIC_STATS=true -e WG_DEFAULT_DNS=1.1.1.1,1.0.0.1 -e WG_PERSISTENT_KEEPALIVE=25 -v /opt/awg-easy/wireguard:/etc/amnezia/amneziawg -v /opt/awg-easy/wireguard:/etc/wireguard --cap-add=NET_ADMIN --cap-add=SYS_MODULE --device /dev/net/tun:/dev/net/tun --restart unless-stopped ghcr.io/yokitoki/awg-easy:latest
sleep 10
test "$(docker inspect -f '{{.State.Health.Status}}' amnezia-wg-easy)" = healthy
wget -q -T 10 --spider "http://__HOST__:__WEB_PORT__/"
docker exec amnezia-wg-easy awg show >/dev/null
printf 'RESULT_UI=http://__HOST__:__WEB_PORT__\nRESULT_PASSWORD=%s\n' "$WEB_PASSWORD"
'@
    $remote = $remote.Replace('__HOST__', $HostName).Replace('__VPN_PORT__', "$VpnPort").Replace('__WEB_PORT__', "$WebPort")
    $result = & ssh -o ConnectTimeout=15 -o StrictHostKeyChecking=accept-new "$SshUser@$HostName" $remote 2>&1
    $result | ForEach-Object { Write-Host $_ }
    if ($LASTEXITCODE -ne 0) { throw "Удалённая команда завершилась с кодом $LASTEXITCODE." }
    $ui = ($result | Select-String '^RESULT_UI=').ToString().Replace('RESULT_UI=', '')
    $uiPassword = ($result | Select-String '^RESULT_PASSWORD=').ToString().Replace('RESULT_PASSWORD=', '')
    if (-not $ui -or -not $uiPassword) { throw 'Удалённая проверка не вернула адрес панели или пароль.' }
    Write-Host "`nГотово. Панель: $ui" -ForegroundColor Green
    Write-Host "Пароль панели: $uiPassword" -ForegroundColor Yellow
    Write-Host "VPN: UDP $VpnPort. Создайте клиента через панель, затем импортируйте QR-код или .conf." 
}
finally {
    Remove-Item -LiteralPath $askPass -Force -ErrorAction SilentlyContinue
    Remove-Item Env:AWG_SSH_PASSWORD -ErrorAction SilentlyContinue
    Remove-Item Env:SSH_ASKPASS -ErrorAction SilentlyContinue
    Remove-Item Env:SSH_ASKPASS_REQUIRE -ErrorAction SilentlyContinue
}
