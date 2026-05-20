#!/bin/bash
#
# exitnode bootstrap script. Fetched at first boot via the GCP startup
# script (instance metadata key `startup-script-url`). Idempotent —
# safe to re-run.
#
# Reads required parameters from VM instance metadata:
#   - tailscale-auth-key  (ephemeral, single-use, ~5m TTL)
#   - tailscale-hostname  (vpn-<region>-<rand>)
#   - tailscale-tags      (comma-separated, e.g. "tag:exit-node")
#
set -euo pipefail

META="http://metadata.google.internal/computeMetadata/v1/instance/attributes"

get_metadata() {
  curl --silent --show-error --fail \
    --header "Metadata-Flavor: Google" \
    "${META}/$1"
}

AUTH_KEY="$(get_metadata tailscale-auth-key)"
HOSTNAME_VAL="$(get_metadata tailscale-hostname)"
TAGS="$(get_metadata tailscale-tags)"

# Validate — fail loudly if any required key is missing.
: "${AUTH_KEY:?tailscale-auth-key metadata missing}"
: "${HOSTNAME_VAL:?tailscale-hostname metadata missing}"
: "${TAGS:?tailscale-tags metadata missing}"

apt-get update
apt-get install -y curl

# Tailscale via official installer (handles repo + key + apt install).
if ! command -v tailscale >/dev/null 2>&1; then
  curl -fsSL https://tailscale.com/install.sh | sh
fi

# IP forwarding — idempotent drop-in; do NOT edit /etc/sysctl.conf.
cat >/etc/sysctl.d/99-tailscale.conf <<EOF
net.ipv4.ip_forward = 1
net.ipv6.conf.all.forwarding = 1
EOF
sysctl --system

# Bring tailscale up. If already up, this re-applies the settings.
tailscale up \
  --auth-key="$AUTH_KEY" \
  --advertise-exit-node \
  --hostname="$HOSTNAME_VAL" \
  --advertise-tags="$TAGS" \
  --ssh

echo "exitnode install.sh: complete"
