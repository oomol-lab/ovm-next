package main

import (
	"context"
	"linuxvm/pkg/define"
	"linuxvm/pkg/librevm"

	"github.com/urfave/cli/v3"
)

var startDocker = cli.Command{
	Name:                      define.SubCommandRun,
	Usage:                     "start ovm podman engine",
	DisableSliceFlagSeparator: true,
	Flags: []cli.Flag{
		&cli.Int8Flag{
			Name:  define.FlagCPUS,
			Usage: "number of vCPU cores to assign to the VM; defaults to host CPU count if unset or less than 1",
		},
		&cli.Uint64Flag{
			Name:  define.FlagMemoryInMB,
			Usage: "VM memory size in MB; minimum 512 MB; defaults to host available memory if unset or less than 512",
		},
		&cli.StringSliceFlag{
			Name:  define.FlagEnvs,
			Usage: "environment variables to pass to the guest process (format: KEY=VALUE); can be specified multiple times",
		},
		&cli.StringSliceFlag{
			Name:  define.FlagRawDisk,
			Usage: "attach an ext4 raw disk image to the VM (format: <path>[,version=<v>][,uuid=<u>][,mnt=<guest-path>]); defaults: version=define.DefaultRawDiskVersion, uuid=random, mnt=/mnt/<UUID>; existing raw disk always keeps its own UUID; can be specified multiple times",
		},
		&cli.StringSliceFlag{
			Name:  define.FlagMount,
			Usage: "share a host directory into the guest via VirtIO-FS (format: /host/path:/guest/path[,ro]); can be specified multiple times",
		},
		&cli.BoolFlag{
			Name:  define.FlagUsingSystemProxy,
			Usage: "read the macOS system HTTP/HTTPS proxy and forward it to the guest as http_proxy/https_proxy env vars; in gvisor mode, 127.0.0.1 is automatically rewritten to host.containers.internal",
		},
		&cli.StringFlag{
			Name:  define.FlagVNetworkType,
			Usage: "virtual network stack: gvisor uses gvisor-tap-vsock (full TCP/UDP, DNS, NAT via 192.168.127.0/24); tsi uses libkrun transparent socket interception",
			Value: string(define.GVISOR),
		},
		&cli.StringFlag{
			Name:  define.FlagReportEvents,
			Usage: "HTTP endpoint to receive VM lifecycle events (e.g. unix:///var/run/events.sock or tcp://192.168.1.252:8888)",
		},
		&cli.StringFlag{
			Name:  define.FlagLogLevel,
			Usage: "log verbosity level (trace, debug, info, warn, error, fatal, panic)",
			Value: "info",
		},
		&cli.StringFlag{
			Name:  define.FlagLogTo,
			Usage: "custom log file path on host; defaults to /tmp/<session_id>/logs/ovm.log when unset",
		},
		&cli.StringFlag{
			Name:  define.FlagSessionID,
			Usage: "session name; used to derive the workspace directory (/tmp/<session_id>); sessions with the same name are mutually exclusive via flock",
		},
		&cli.StringFlag{
			Name:  define.FlagVarDisk,
			Usage: "path/version for guest /var raw disk (format: <path>[,version=<v>]); defaults: version=define.DefaultRawDiskVersion, uuid=define.VarDataDiskUUID, mnt=/var; auto-created if the file does not exist",
		},
		&cli.StringFlag{
			Name:  define.FlagPodmanProxyAPIFile,
			Usage: "custom Unix socket path for the host-side Podman API proxy; defaults to /tmp/<session_id>/socks/podman-api.sock",
		},
		&cli.StringFlag{
			Name:  define.FlagManageAPIFile,
			Usage: "custom Unix socket path for the host-side VM management API; defaults to /tmp/<session_id>/socks/vmctl.sock",
		},
		&cli.StringFlag{
			Name:  define.FlagExportSSHKeyPrivateFile,
			Usage: "file path to symlink the generated SSH private key to",
		},
		&cli.StringFlag{
			Name:  define.FlagExportSSHKeyPublicFile,
			Usage: "file path to symlink the generated SSH public key to",
		},
		// legacy hidden flags set
		&cli.StringFlag{
			Name:   define.FlagOVMWorkspace,
			Usage:  "not use any more, retained for compatibility",
			Hidden: true,
		},
		&cli.Uint64Flag{
			Name:   define.FlagOVMPPID,
			Usage:  "not use any more, retained for compatibility",
			Hidden: true,
		},
		&cli.StringFlag{
			Name:   define.FlagOVMName,
			Usage:  "not use any more, retained for compatibility",
			Hidden: true,
		},
		&cli.StringFlag{
			Name:   define.FlagOVMReportURL,
			Usage:  "legacy event, for ovm-js compatibility, use --report-events instead",
			Hidden: true,
		},
	},
	Action: dockerLifeCycle,
}

func dockerLifeCycle(_ context.Context, command *cli.Command) error {
	// Shield the upper-level ctx to prevent upstream ctx from causing unexpected VM exit
	// To safely stop the virtual machine, cancel() should be called
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg := librevm.DefaultConfig(command.String(define.FlagSessionID)).
		WithLogLevelAndLogFile(command.String(define.FlagLogLevel), command.String(define.FlagLogTo)).
		WithMode(librevm.ModeContainer).
		WithCPUs(int(command.Int8(define.FlagCPUS))).
		WithMemory(command.Uint64(define.FlagMemoryInMB)).
		WithNetwork(command.String(define.FlagVNetworkType)).
		WithProxy(command.Bool(define.FlagUsingSystemProxy)).
		WithEnv(command.StringSlice(define.FlagEnvs)...).
		WithDisk(command.StringSlice(define.FlagRawDisk)...).
		WithMount(command.StringSlice(define.FlagMount)...).
		WithVarDataDisk(command.String(define.FlagVarDisk)).
		WithPodmanProxyAPIFile(command.String(define.FlagPodmanProxyAPIFile)).
		WithManageAPIFile(command.String(define.FlagManageAPIFile)).
		WithExportSSHKeyPrivateFile(command.String(define.FlagExportSSHKeyPrivateFile)).
		WithExportSSHKeyPublicFile(command.String(define.FlagExportSSHKeyPublicFile)).
		WithReportEndpoint(command.String(define.FlagReportEvents))

	cfg.WithReportEndpoint(command.String(define.FlagOVMReportURL)) // compatibility

	if overwriteCfg, err := librevm.LoadFile(vmConfigFilePath); err == nil {
		cfg.OverwriteCfgFrom(overwriteCfg)
	}

	vm, err := librevm.New(cfg)
	if err != nil {
		return err
	}

	vm.Cancel = cancel

	defer vm.Close()

	return vm.RunDocker(ctx)
}
