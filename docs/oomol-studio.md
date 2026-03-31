现阶段 oomol-studio 的 ovm 启动分两个阶段

# init

```shell
ovm \
    init --cpus 7 --memory 22528 \
    --boot "/Applications/OOMOL Studio.app/Contents/Resources/app/node_modules/@oomol-lab/ovm/vm-resources/bootable.img.zst" \
    --boot-version 0.0.12 \
    --data-version 0.0.33 \
    --report-url unix:///var/folders/dt/wqkv0wf13nl6n8jbf98jggjh0000gn/T/ovm-1uvWyo/event-restful-init.sock \
    --workspace /Users/danhexon/.oomol-studio/ovm-krun \
    --ppid 29896 \
    --volume /Users:/Users \
    -name default \
    --volume /Applications/OOMOL Studio.app/Contents/Resources/app/container-resources:/Applications/OOMOL Studio.app/Contents/Resources/app/container-resources \
    --volume /Applications/OOMOL Studio.app/Contents/Resources/app/resources:/Applications/OOMOL Studio.app/Contents/Resources/app/resources
```

# start

```shell
ovm start \
    --report-url unix:///var/folders/dt/wqkv0wf13nl6n8jbf98jggjh0000gn/T/ovm-1uvWyo/event-restful-run.sock \
    --workspace /Users/danhexon/.oomol-studio/ovm-krun \
    --ppid 29896 \
    -name default
```

ovm-next 兼容了这种启动模式，当其本质不同

- ovm init 直接变成了存配置生成器，生成一个 json 文件（`/tmp/vmcfg-afd8c036e065.json`），规定了 vm 应该如何配置和启动
- ovm start 通过 apply `/tmp/vmcfg-afd8c036e065.json` 来加载这些配置，启动 vm

# 兼容性

ovm 的 init & start 都需要满足基本的 flag 兼容：

- ovm init 的 flag 完全兼容老版本的 ovm init 参数
- ovm start 的 flag 也需要兼容老版本的 ovm start 参数，只要不报错就行