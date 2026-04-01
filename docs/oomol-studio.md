# OOMOL Studio 兼容说明（ovm-next）

本文定义 oomol-studio 调用 `ovm` 的兼容规则，目标是：

- 保持现有调用方式可继续使用
- 明确哪些参数“仅兼容解析”与“实际生效”
- 明确参数覆盖优先级，避免行为歧义

## 1. 启动模型

oomol-studio 仍按两阶段调用：

1. `ovm init`：生成 VM 配置文件（`/tmp/vmcfg-afd8c036e065.json`）
2. `ovm start`：加载该配置并启动 VM

即：`init` 负责“写配置”，`start` 负责“运行配置”。

## 2. 兼容目标

### 2.1 init 兼容目标

兼容 oomol-studio 现有 `ovm init` 参数集合：

- 参数可以按旧方式传入
- 不因参数本身报错
- 仅部分参数参与生成 vmcfg（见“生效字段清单”）

### 2.2 start 兼容目标

兼容 oomol-studio 现有 `ovm start` 参数集合，尤其 legacy 参数：

- `--report-url`
- `--workspace`
- `--ppid`
- `-name`

这些参数允许继续传入；即使被忽略，也不能因为参数本身导致失败。

## 3. 生效字段清单

### 3.1 `ovm init`：哪些参数会写入 vmcfg

会写入（生效）：

- `--cpus` -> `cpus`
- `--memory` -> `memoryMB`
- `--data-version` -> `varDisk.version`
- `--volume` -> `mounts`
- `--report-url` -> `reportURL`
- `--workspace` 与 `-name` -> 共同决定 `baseDir`，用于生成：
  - `externalDisks` / `varDisk` 路径
  - `logTo`
  - `podmanProxyAPIFile`
  - `manageAPIFile`
  - ssh key symlink 路径

仅兼容解析（当前不参与 vmcfg 关键决策）：

- `--boot`
- `--boot-version`
- `--ppid`

### 3.2 `ovm start`：哪些参数会影响运行态配置

会生效（若传入）：

- `--id`
- `--cpus`
- `--memory`
- `--network`
- `--system-proxy`
- `--envs`
- `--raw-disk`
- `--mount`
- `--var-disk`
- `--podman-api`
- `--manage-api`
- `--ssh-private-key`
- `--ssh-public-key`
- `--log-level`
- `--log-to`
- `--report-events`（新参数）
- `--report-url`（legacy 参数）

仅兼容解析（当前可忽略）：

- `--workspace`
- `--ppid`
- `-name`

## 4. 覆盖优先级（关键规则）

`ovm start` 的配置来源按以下顺序处理：

1. 先加载 `init` 生成的 vmcfg（若存在）
2. 再应用 `start` 传入参数覆盖

事件上报地址优先级：

1. `--report-events`（新）
2. `--report-url`（legacy，兜底）

即：新参数优先，旧参数兜底。

## 5. “只要不报错”的准确含义

这里的“只要不报错”，特指 legacy `ovm start` 兼容：

- 旧参数可以继续传入
- 旧参数可以被忽略（不影响最终运行配置）
- 但不能因为这些参数本身触发解析错误或启动失败

## 6. 参考调用

### 6.1 init

```shell
ovm init --cpus 7 --memory 22528 \
  --boot "/Applications/OOMOL Studio.app/.../bootable.img.zst" \
  --boot-version 0.0.12 \
  --data-version 0.0.33 \
  --report-url unix:///.../event-restful-init.sock \
  --workspace /Users/danhexon/.oomol-studio/ovm-krun \
  --ppid 29896 \
  --volume /Users:/Users \
  -name default
```

### 6.2 start

```shell
ovm start \
  --report-url unix:///.../event-restful-run.sock \
  --workspace /Users/danhexon/.oomol-studio/ovm-krun \
  --ppid 29896 \
  -name default
```
