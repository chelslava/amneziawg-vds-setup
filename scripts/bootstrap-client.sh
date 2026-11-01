#!/usr/bin/env bash
# Runs ON the runner; hops to the VDS over SSH and finishes the deployment:
# hardens the firewall and provisions the first VPN client via the panel API.
# Secrets never reach stdout in plaintext: the client config and panel
# password are printed only as AES-256-CBC blobs encrypted with CLIENT_PASSWORD.
set -eu
HOST=$1
SSH_PORT=$2
PANEL_IP=$3
HOME_IP=$4
CLIENT_NAME=$5
CLIENT_PASSWORD=$6

SSHOPT=(-p "$SSH_PORT" -i "$HOME/.ssh/id_awg" -o UserKnownHostsFile=./known_hosts -o StrictHostKeyChecking=yes -o BatchMode=yes)

ssh "${SSHOPT[@]}" root@"$HOST" bash -s -- "$PANEL_IP" "$HOME_IP" "$CLIENT_NAME" "$CLIENT_PASSWORD" <<'REMOTE'
set -eu
PANEL_IP=$1
HOME_IP=$2
CLIENT_NAME=$3
CLIENT_PASSWORD=$4

echo "=== firewall:"
export DEBIAN_FRONTEND=noninteractive
command -v ufw >/dev/null || { apt-get update -qq >/dev/null 2>&1; apt-get install -y -qq ufw >/dev/null 2>&1; }
ufw default deny incoming >/dev/null
ufw default allow outgoing >/dev/null
ufw allow 22/tcp >/dev/null
ufw allow 443/tcp >/dev/null
ufw allow 1234/udp >/dev/null
ufw insert 1 allow from "$PANEL_IP" to any port 51821 proto tcp >/dev/null
ufw insert 2 allow from "$HOME_IP" to any port 51821 proto tcp >/dev/null
ufw insert 3 deny 51821/tcp >/dev/null
ufw route allow in on wg0 >/dev/null
ufw --force enable >/dev/null
ufw status verbose | head -14

echo "=== client:"
PW=$(cat /opt/awg-vds/panel-password)
curl -s -c /tmp/awg-cookie -X POST http://127.0.0.1:51821/api/auth/password \
  -H 'Content-Type: application/json' \
  -d "{\"username\":\"admin\",\"password\":\"$PW\",\"remember\":false}" >/dev/null
CREATE=$(curl -s -b /tmp/awg-cookie -X POST http://127.0.0.1:51821/api/client \
  -H 'Content-Type: application/json' -d "{\"name\":\"$CLIENT_NAME\",\"expiresAt\":null,\"enabled\":true}")
echo "CREATE_RESP=$CREATE"
CID=$(printf '%s' "$CREATE" | grep -oE '"id":"[^"]+"' | head -1 | cut -d'"' -f4)
if [ -z "$CID" ]; then
  CID=$(curl -s -b /tmp/awg-cookie http://127.0.0.1:51821/api/client | grep -oE '"id":"[^"]+"' | head -1 | cut -d'"' -f4)
fi
echo "CLIENT_ID=$CID"
test -n "$CID"
CONF=""
for p in configuration config ""; do
  CONF=$(curl -s -b /tmp/awg-cookie "http://127.0.0.1:51821/api/client/$CID/$p")
  case "$CONF" in
    *'[Interface]'*) printf '%s\n' "$CONF" > /tmp/client.conf; echo "CONF_VIA=$p"; break ;;
  esac
  CONF=""
done
if [ ! -s /tmp/client.conf ]; then
  docker exec awg-vds-upstream ls /app/server/chunks/routes/api/client/ || true
  echo "CONF_NOT_FOUND"
  exit 3
fi
SRV=$(grep '^WG_HOST=' /opt/awg-vds/upstream.env | cut -d= -f2)
PORT=$(grep '^WG_PORT=' /opt/awg-vds/upstream.env | cut -d= -f2)
sed -i "s#Endpoint = .*#Endpoint = ${SRV}:${PORT}#" /tmp/client.conf
openssl enc -aes-256-cbc -pbkdf2 -salt -a -in /tmp/client.conf -pass "pass:$CLIENT_PASSWORD" -out /tmp/client.enc
echo '--- CLIENT_CONF_ENCRYPTED_BASE64_BELOW ---'
cat /tmp/client.enc
echo '--- END ---'
printf 'PANEL_PASSWORD_B64=%s\n' "$(printf '%s' "$PW" | openssl enc -aes-256-cbc -pbkdf2 -salt -a -pass "pass:$CLIENT_PASSWORD" | tr -d '\n')"
rm -f /tmp/awg-cookie /tmp/client.conf /tmp/client.enc
REMOTE
echo "BOOTSTRAP_CLIENT_DONE"
