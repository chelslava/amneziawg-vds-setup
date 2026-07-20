# =====================================================================================
# Install-AmneziaWG.ps1
# Назначение: удалённая установка/обновление/переконфигурация AmneziaWG (WireGuard
# с обфускацией) на VDS под Ubuntu/Debian через SSH из PowerShell.
# Скрипт подключается по SSH (по ключу или паролю), генерирует случайные параметры
# обфускации (JC/JMIN/JMAX/S1/S2/H1-H4), поднимает Docker-контейнеры awg-easy и
# (опционально) Caddy для TLS, настраивает UFW и проверяет, что сервис поднялся
# и отвечает.
# =====================================================================================
#requires -Version 7.0
[CmdletBinding()]
param(
 [string]$HostName, [string]$SshUser = 'root', [int]$VpnPort = 1234, [int]$WebPort = 51821,
 [ValidateSet('Install','Update','Status','Reconfigure')][string]$Mode = 'Install',
 [string]$IdentityFile, [PSCredential]$SshCredential, [string]$TlsDomain, [switch]$DisableTls, [switch]$ConfigureUfw,
 [switch]$RestrictPanelToTls, [switch]$Force
)
Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
function Need($Name, $Value) { if ([string]::IsNullOrWhiteSpace($Value)) { throw "$Name is required." } }
function Result($Lines, $Name) { $p="$Name="; $l=$Lines | % { "$_" } | ? { $_.StartsWith($p) } | select -Last 1; if ($null -ne $l) { $l.Substring($p.Length) } }
function Check-External($Uri) { try { $r=Invoke-WebRequest $Uri -TimeoutSec 15 -SkipHttpErrorCheck; if($r.StatusCode -eq 200){Write-Host "Внешняя проверка: HTTP 200 ($Uri)" -ForegroundColor Green}else{Write-Warning "Внешняя проверка вернула HTTP $($r.StatusCode): $Uri"} }catch{Write-Warning "Внешняя проверка не удалась: $($_.Exception.Message). Проверьте DNS и firewall VDS/провайдера."} }
if(!$HostName){$HostName=Read-Host 'IP-адрес или DNS-имя VDS'}
Need HostName $HostName
if($HostName -notmatch '^[a-zA-Z0-9.-]+$' -or $HostName.StartsWith('.') -or $HostName.EndsWith('.')){throw 'Недопустимое имя хоста.'}
if($SshUser -notmatch '^[a-z_][a-z0-9_-]*$'){throw 'Недопустимое имя SSH-пользователя.'}
if($VpnPort -notin 1..65535 -or $WebPort -notin 1..65535 -or $VpnPort -eq $WebPort){throw 'Проверьте порты.'}
if($TlsDomain -and $TlsDomain -notmatch '^(?!-)[a-zA-Z0-9-]+(?:\.[a-zA-Z0-9-]+)+$'){throw 'TlsDomain должен быть доменом, а не IP.'}
if($DisableTls -and $TlsDomain){throw 'Нельзя использовать TlsDomain и DisableTls вместе.'}
if($RestrictPanelToTls -and !$TlsDomain){throw 'RestrictPanelToTls требует TlsDomain.'}
if($Mode -eq 'Reconfigure' -and !$Force){throw 'Reconfigure сбрасывает пароль панели. Повторите с -Force.'}
if($Mode -ne 'Reconfigure' -and $Force){throw 'Force используется только с Reconfigure.'}
if(!(Get-Command ssh -EA SilentlyContinue)){throw 'Не найден OpenSSH Client.'}
$password=$null
if($IdentityFile){$IdentityFile=$ExecutionContext.SessionState.Path.GetUnresolvedProviderPathFromPSPath($IdentityFile); if(!(Test-Path -LiteralPath $IdentityFile -PathType Leaf)){throw "Не найден SSH-ключ: $IdentityFile"}}
else{if($SshCredential){if($SshCredential.UserName -ne $SshUser){throw 'SshCredential user must match SshUser.'};$password=$SshCredential.GetNetworkCredential().Password}else{$password=(Get-Credential -UserName $SshUser -Message "SSH-пароль для $SshUser@$HostName").GetNetworkCredential().Password};Need 'SSH password' $password}
$names='AWG_SSH_PASSWORD','SSH_ASKPASS','SSH_ASKPASS_REQUIRE','DISPLAY'; $old=@{}; foreach($n in $names){$i=Get-Item "Env:$n" -EA SilentlyContinue;$old[$n]=if($i){$i.Value}else{$null}}; $ask=$null
try {
 $opt=@('-o','ConnectTimeout=15','-o','StrictHostKeyChecking=accept-new')
 if($IdentityFile){$opt+=@('-o','BatchMode=yes','-o','IdentitiesOnly=yes','-i',$IdentityFile)}else{$ask=Join-Path $env:TEMP "awg-askpass-$([guid]::NewGuid()).cmd";Set-Content $ask @('@echo off','echo %AWG_SSH_PASSWORD%') -Encoding ascii;$env:AWG_SSH_PASSWORD=$password;$env:SSH_ASKPASS=$ask;$env:SSH_ASKPASS_REQUIRE='force';$env:DISPLAY='1'}
 Write-Host 'Проверяю SSH-доступ и ключ хоста...' -ForegroundColor Cyan; & ssh @opt "$SshUser@$HostName" true; if($LASTEXITCODE){throw "SSH-предпроверка завершилась с кодом $LASTEXITCODE."}
$remote=@'
set -eu
MODE='__MODE__'; HOST='__HOST__'; VPN='__VPN__'; WEB='__WEB__'; TLS='__TLS__'; UFW='__UFW__'; RESTRICT='__RESTRICT__'; DISABLE='__DISABLE__'
[ "$(id -u)" = 0 ] || { echo 'ERROR=SSH user must be root.' >&2; exit 1; }
. /etc/os-release; case "$ID" in ubuntu|debian);; *) echo "ERROR=Unsupported OS: $ID" >&2; exit 1;; esac
exists(){ test -f /opt/awg-easy/.env; }; packages(){ apt-get update; apt-get install -y docker.io apache2-utils openssl curl; systemctl enable --now docker; }
sysctl_apply(){ printf '%s\n' net.ipv4.ip_forward=1 net.ipv4.conf.all.src_valid_mark=1 >/etc/sysctl.d/99-amneziawg.conf;sysctl --system >/dev/null; }
rand_range(){ min="$1";max="$2";printf '%s' "$(( min + $(od -An -N4 -tu4 /dev/urandom) % (max - min + 1) ))"; }
generate_awg_params(){
  JC="$(rand_range 3 6)";JMIN="$(rand_range 40 89)";JMAX="$(( JMIN + $(rand_range 50 250) ))";S1="$(rand_range 15 150)";S2="$(rand_range 15 150)";
  while [ "$S2" = "$((S1 + 56))" ];do S2="$(rand_range 15 150)";done
  H1="$(rand_range 100000000 4294967294)";H2="$(rand_range 100000000 4294967294)";H3="$(rand_range 100000000 4294967294)";H4="$(rand_range 100000000 4294967294)";
  while [ "$H2" = "$H1" ];do H2="$(rand_range 100000000 4294967294)";done
  while [ "$H3" = "$H1" ]||[ "$H3" = "$H2" ];do H3="$(rand_range 100000000 4294967294)";done
  while [ "$H4" = "$H1" ]||[ "$H4" = "$H2" ]||[ "$H4" = "$H3" ];do H4="$(rand_range 100000000 4294967294)";done
}
write_env(){ p="$1";dev="$(ip route show default|awk '{print $5;exit}')";h="$(htpasswd -nbBC 12 '' "$p"|cut -d: -f2)";umask 077;t="$(mktemp /opt/awg-easy/.env.XXXXXX)";printf 'WG_HOST=%s\nWG_PORT=%s\nWEB_PORT=%s\nWG_DEVICE=%s\nPASSWORD_HASH=%s\nJC=%s\nJMIN=%s\nJMAX=%s\nS1=%s\nS2=%s\nH1=%s\nH2=%s\nH3=%s\nH4=%s\n' "$HOST" "$VPN" "$WEB" "$dev" "$h" "$JC" "$JMIN" "$JMAX" "$S1" "$S2" "$H1" "$H2" "$H3" "$H4" >"$t";chmod 600 "$t";mv "$t" /opt/awg-easy/.env; }
run(){ docker pull ghcr.io/yokitoki/awg-easy:latest;docker rm -f amnezia-wg-easy >/dev/null 2>&1||true;docker run -d --name amnezia-wg-easy --network host --env-file /opt/awg-easy/.env -e LANG=ru -e UI_TRAFFIC_STATS=true -e WG_DEFAULT_DNS=1.1.1.1,1.0.0.1 -e WG_PERSISTENT_KEEPALIVE=25 -v /opt/awg-easy/wireguard:/etc/amnezia/amneziawg -v /opt/awg-easy/wireguard:/etc/wireguard --cap-add=NET_ADMIN --cap-add=SYS_MODULE --device /dev/net/tun:/dev/net/tun --restart unless-stopped ghcr.io/yokitoki/awg-easy:latest >/dev/null; }
tls(){ if [ -n "$TLS" ];then install -d -m 700 /opt/awg-easy/caddy-data /opt/awg-easy/caddy-config;printf '%s {\n reverse_proxy 127.0.0.1:%s\n}\n' "$TLS" "$WEB">/opt/awg-easy/Caddyfile;docker pull caddy:2-alpine;docker rm -f amnezia-wg-caddy >/dev/null 2>&1||true;docker run -d --name amnezia-wg-caddy --network host -v /opt/awg-easy/Caddyfile:/etc/caddy/Caddyfile:ro -v /opt/awg-easy/caddy-data:/data -v /opt/awg-easy/caddy-config:/config --restart unless-stopped caddy:2-alpine >/dev/null;elif [ "$DISABLE" = true ];then docker rm -f amnezia-wg-caddy >/dev/null 2>&1||true;fi; }
firewall(){ [ "$UFW" = true ]||{ echo 'WARNING=Firewall was not modified; open required VDS/provider ports.';return;};command -v ufw >/dev/null||{ echo 'WARNING=UFW is unavailable.';return;};[ "$(ufw status|head -1)" = 'Status: active' ]||{ echo 'WARNING=UFW is inactive.';return;};ufw allow "$VPN/udp";if [ -n "$TLS" ];then ufw allow 80/tcp;ufw allow 443/tcp;[ "$RESTRICT" = true ]&&ufw deny "$WEB/tcp"||true;else ufw allow "$WEB/tcp";fi; }
verify(){ h=;n=0;until [ "$h" = healthy ]||[ "$n" -ge 12 ];do h="$(docker inspect -f '{{.State.Health.Status}}' amnezia-wg-easy 2>/dev/null||true)";[ "$h" = healthy ]&&break;n=$((n+1));sleep 5;done;test "$h" = healthy;test "$(curl -sS -o /dev/null -w '%{http_code}' --max-time 10 "http://127.0.0.1:$WEB/")" = 200;docker exec amnezia-wg-easy awg show >/dev/null;ss -ltnH|grep -Eq "[:.]$WEB([[:space:]]|$)";ss -lunH|grep -Eq "[:.]$VPN([[:space:]]|$)"; }
case "$MODE" in Install) exists&&{ echo 'ERROR=Existing installation found. Use Update, Status, or Reconfigure -Force.'>&2;exit 1;};packages;sysctl_apply;install -d -m 700 /opt/awg-easy/wireguard;generate_awg_params;p="$(openssl rand -hex 16)";write_env "$p";run;tls;firewall;verify;echo "RESULT_UI=http://$HOST:$WEB";echo "RESULT_PASSWORD=$p";; Update) exists||{ echo 'ERROR=No installation found.'>&2;exit 1;};packages;sysctl_apply;run;tls;firewall;verify;echo "RESULT_UI=http://$HOST:$WEB";; Reconfigure) exists||{ echo 'ERROR=No installation found.'>&2;exit 1;};packages;sysctl_apply;generate_awg_params;p="$(openssl rand -hex 16)";write_env "$p";run;tls;firewall;verify;echo "RESULT_UI=http://$HOST:$WEB";echo "RESULT_PASSWORD=$p";; Status) exists||{ echo 'ERROR=No installation found.'>&2;exit 1;};verify;echo "RESULT_UI=http://$HOST:$WEB";echo RESULT_STATUS=healthy;; esac
'@
$remote = $remote.Replace('__MODE__',$Mode).Replace('__HOST__',$HostName).Replace('__VPN__',"$VpnPort").Replace('__WEB__',"$WebPort").Replace('__TLS__',$TlsDomain).Replace('__UFW__',$ConfigureUfw.IsPresent.ToString().ToLower()).Replace('__RESTRICT__',$RestrictPanelToTls.IsPresent.ToString().ToLower()).Replace('__DISABLE__',$DisableTls.IsPresent.ToString().ToLower())
# Нормализуем переносы строк на Unix-стиль (LF), чтобы избежать проблем при кодировании
$remote = $remote -replace "`r`n", "`n"

# ВАЖНО: скрипт передаётся на сервер не через прямой конвейер "| ssh ... bash -s",
# а закодированным в Base64. Прямая передача многострочного текста через конвейер
# PowerShell -> ssh -> bash на Windows может искажаться (лишние \r, особенности
# буферизации/кодировки консоли), из-за чего bash получает повреждённый скрипт и
# падает с ошибкой вида "syntax error near unexpected token `newline'" (код возврата 2).
# Base64 состоит только из ASCII-символов (A-Z a-z 0-9 + / =), поэтому такая передача
# гарантированно не искажается независимо от кодировки консоли и настроек конвейера.
$remoteBytes = [System.Text.Encoding]::UTF8.GetBytes($remote)
$remoteBase64 = [Convert]::ToBase64String($remoteBytes)
# На сервере строка декодируется обратно в текст скрипта и передаётся в bash через stdin
$remoteCommand = "echo '$remoteBase64' | base64 -d | bash -s"

# Единственный запуск удалённого скрипта.
# (Ранее здесь был повторный вызов той же команды строкой ниже — это была ошибка
# копирования: при успешном первом запуске вся установка/настройка выполнялась
# на сервере ЕЩЁ РАЗ, что для режима Install приводило к ошибке "уже установлено".
# Дублирующий блок удалён.)
$out = & ssh @opt "$SshUser@$HostName" $remoteCommand 2>&1
$out | Where-Object { $_ -notmatch '^RESULT_PASSWORD=' } | ForEach-Object { Write-Host $_ }
if ($LASTEXITCODE) { throw "Удалённая команда завершилась с кодом $LASTEXITCODE." }
$ui = Result $out RESULT_UI
if (!$ui) { throw 'Удалённая проверка не вернула адрес панели.' }
$public = if ($TlsDomain) { "https://$TlsDomain" } else { $ui }
Check-External $public
Write-Host "`nГотово. Панель: $public" -ForegroundColor Green
$panelPassword = Result $out RESULT_PASSWORD
if ($panelPassword) { Write-Host "Пароль панели: $panelPassword" -ForegroundColor Yellow }
Write-Host "VPN: UDP $VpnPort."
} finally {if($ask){Remove-Item $ask -Force -EA SilentlyContinue};foreach($n in $names){if($null -eq $old[$n]){Remove-Item "Env:$n" -EA SilentlyContinue}else{Set-Item "Env:$n" $old[$n]}}}
