# revm

[libkrun](https://github.com/containers/libkrun) 驱动的轻量级 Linux 微虚拟机管理器。在 macOS/arm64 和 Linux/(arm64|amd64) 上启动 Linux 客户机，支持 Podman 兼容容器引擎。

## 子命令

### `start` — 启动虚拟机与 Podman 引擎

```
revm start [flags]
```

启动一个运行 Podman 兼容容器引擎的微虚拟机。进程退出时虚拟机随之关闭。

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `--cpus` | int | 宿主机 CPU 数 | vCPU 核心数 |
| `--memory` | uint64 | 宿主机可用内存 | 虚拟机内存（MB），最低 512 |
| `--id` | string | | 会话名称，工作目录为 `/tmp/<id>` |
| `--envs` | string[] | | 环境变量（`KEY=VALUE`），可多次指定 |
| `--raw-disk` | string[] | | 挂载 ext4 裸磁盘（`<路径>[,uuid]`），可多次指定 |
| `--mount` | string[] | | VirtIO-FS 共享目录（`/宿主路径:/客户路径[,ro]`），可多次指定 |
| `--container-disk` | string | | 容器存储用持久化 ext4 磁盘 |
| `--container-disk-version` | string | | 容器磁盘版本标识，版本变更时触发磁盘格式升级 |
| `--network` | string | `gvisor` | 虚拟网络：`gvisor`（NAT, 192.168.127.0/24）或 `tsi`（透明套接字拦截） |
| `--system-proxy` | bool | false | 将 macOS 系统 HTTP/HTTPS 代理转发到客户机 |
| `--podman-proxy-api-file` | string | `/tmp/<id>/socks/podman-api.sock` | 宿主机侧 Podman API Unix 套接字路径 |
| `--manage-api-file` | string | `/tmp/<id>/socks/vmctl.sock` | 虚拟机管理 API Unix 套接字路径 |
| `--ssh-private-key` | string | | SSH 私钥符号链接路径 |
| `--ssh-public-key` | string | | SSH 公钥符号链接路径 |
| `--report-events` | string | | 生命周期事件 HTTP 端点（`unix:///路径` 或 `tcp://地址:端口`） |
| `--log-level` | string | `info` | 日志级别：trace, debug, info, warn, error, fatal, panic |
| `--log-to` | string | `/tmp/<id>/logs/vm.log` | 自定义日志文件路径 |

#### 示例

```bash
# 4 核 2 GB 内存启动
revm start --cpus 4 --memory 2048 --id my-session

# 只读挂载宿主目录并附加数据盘
revm start --id dev \
  --mount /home/user/src:/workspace,ro \
  --raw-disk /var/lib/data.img

# 转发 macOS 系统代理
revm start --id dev --system-proxy
```

### `attach` — 连接到运行中的虚拟机

```
revm attach [--pty] <session-name> [-- <command> [args...]]
```

通过 SSH 连接到已运行的虚拟机会话。会话名称对应 `/tmp/<name>`。

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `--pty` | bool | false | 分配伪终端，启动交互式 shell |
| `--log-level` | string | `info` | 日志级别 |

#### 示例

```bash
# 交互式 shell
revm attach --pty my-session

# 执行单条命令
revm attach my-session -- ls -la /workspace

# 执行多参数命令
revm attach my-session -- podman ps -a
```

## 平台支持

| 操作系统 | 架构 |
|---------|------|
| macOS | arm64 (Apple Silicon) |
| Linux | arm64, amd64 |
