# ovm

Lightweight Linux microVM manager backed by [libkrun](https://github.com/containers/libkrun). Boots Linux guests on
macOS/arm64 and Linux/(arm64|amd64) using Apple Hypervisor or KVM, with optional Podman-compatible container engine
support.

## Subcommands

### `start` — Start the VM with Podman engine

```
ovm start [flags]
```

Boots a microVM running a Podman-compatible container engine. The VM lifecycle is tied to the process; stopping the
process shuts down the VM.

| Flag                       | Type     | Default                           | Description                                                                                     |
|----------------------------|----------|-----------------------------------|-------------------------------------------------------------------------------------------------|
| `--cpus`                   | int      | host CPU count                    | Number of vCPU cores                                                                            |
| `--memory`                 | uint64   | host available                    | VM memory in MB (min 512)                                                                       |
| `--id`                     | string   |                                   | Session name; workspace is `/tmp/<id>`                                                          |
| `--envs`                   | string[] |                                   | Environment variables (`KEY=VALUE`), repeatable                                                 |
| `--raw-disk`               | string[] |                                   | Attach ext4 disk (`<path>[,uuid]`), repeatable                                                  |
| `--mount`                  | string[] |                                   | VirtIO-FS shared directory (`/host:/guest[,ro]`), repeatable                                    |
| `--data-disk`              | string   |                                   | Persistent ext4 disk for guest `/var`; the raw disk still mounts under `/mnt/<UUID>` first     |
| `--data-disk-version`      | string   |                                   | Version tag for the data disk; triggers disk format upgrade when changed                        |
| `--network`                | string   | `gvisor`                          | Virtual network: `gvisor` (NAT via 192.168.127.0/24) or `tsi` (transparent socket interception) |
| `--system-proxy`           | bool     | false                             | Forward macOS system HTTP/HTTPS proxy to guest                                                  |
| `--podman-api`             | string   | `/tmp/<id>/socks/podman-api.sock` | Unix socket for host-side Podman API                                                            |
| `--manage-api`             | string   | `/tmp/<id>/socks/vmctl.sock`      | Unix socket for VM management API                                                               |
| `--ssh-private-key`        | string   |                                   | Symlink path for generated SSH private key                                                      |
| `--ssh-public-key`         | string   |                                   | Symlink path for generated SSH public key                                                       |
| `--report-url`             | string   |                                   | HTTP endpoint for lifecycle events (`unix:///path` or `tcp://host:port`)                        |
| `--log-level`              | string   | `info`                            | Log verbosity: trace, debug, info, warn, error, fatal, panic                                    |
| `--log-to`                 | string   | `/tmp/<id>/logs/vm.log`           | Custom log file path                                                                            |

#### Examples

```bash
# Start with 4 cores and 2 GB RAM
ovm start --cpus 4 --memory 2048 --id my-session

# Mount a host directory read-only and attach a data disk
ovm start --id dev \
  --mount /home/user/src:/workspace,ro \
  --raw-disk /var/lib/data.img

# Forward macOS system proxy into the guest
ovm start --id dev --system-proxy
```

### `attach` — Attach to a running VM

```
ovm attach [--pty] <session-name> [-- <command> [args...]]
```

Connects to a running VM session via SSH. The session name maps to `/tmp/<name>`.

| Flag          | Type   | Default | Description                                      |
|---------------|--------|---------|--------------------------------------------------|
| `--pty`       | bool   | false   | Allocate a pseudo-terminal for interactive shell |
| `--log-level` | string | `info`  | Log verbosity                                    |

#### Examples

```bash
# Interactive shell
ovm attach --pty my-session

# Run a single command
ovm attach my-session -- ls -la /workspace

# Run a multi-arg command
ovm attach my-session -- podman ps -a
```

## Platform Support

| OS    | Architecture          |
|-------|-----------------------|
| macOS | arm64 (Apple Silicon) |
| Linux | arm64, amd64          |
