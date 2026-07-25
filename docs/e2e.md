# Disposable VDS E2E

`.github/workflows/e2e.yml` is a manual, destructive workflow for the GitHub `e2e` environment. It expects disposable Ubuntu and Debian amd64 hosts already provisioned by the operator or an external provider integration:

- `E2E_UBUNTU_HOST` and `E2E_DEBIAN_HOST`;
- `E2E_SSH_USER`, optional `E2E_SSH_PORT`;
- `E2E_SSH_PRIVATE_KEY` and pinned `E2E_KNOWN_HOSTS`.

The workflow runs `doctor`, fresh and repeated Legacy install, `status`, `backup`, and `update`. It optionally probes the upstream diagnostic path on Debian. An `always()` cleanup removes the managed containers, `/opt/awg-vds`, and stored credentials. Do not use shared or production hosts.

The repository deliberately does not embed a cloud-provider SDK or token. Provisioning and provider-level VM destruction remain an environment integration concern; the workflow's cleanup is the final host-side safety net.
