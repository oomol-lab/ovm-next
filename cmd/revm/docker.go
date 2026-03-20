package main

import (
	"context"
	"linuxvm/pkg/define"
	"linuxvm/pkg/eventreporter"
	"linuxvm/pkg/librevm"

	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v3"
)

var startDocker = cli.Command{
	Name:                      define.SubCommandRun,
	Usage:                     "start ovm podman engine",
	DisableSliceFlagSeparator: true,
	Flags: []cli.Flag{
		cpuFlag,
		memoryFlag,
		envsFlag,
		diskFlag,
		mountFlag,
		proxyFlag,
		networkFlag,
		reportEventsFlag,
		logLevelFlag,
		logToFlag,
		sessionIDFlag,
		&cli.StringFlag{
			Name:  define.FlagContainerDisk,
			Usage: "path to a persistent ext4 raw disk image for container storage; auto-created if the file does not exist; defaults to a workspace-local disk if unset",
		},
		&cli.StringFlag{
			Name:  define.FlagPodmanProxyAPIFile,
			Usage: "custom Unix socket path for the host-side Podman API proxy; defaults to /tmp/<session_id>/socks/podman-api.sock",
		},
		manageAPIFlag,
		sshKeyDirFlag,
		sshPrivateKeyFlag,
		sshPublicKeyFlag,

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
			Usage:  "legacy event, for ovm-js compatibility, use --report-events-to instead",
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

	cfg := librevm.DefaultConfig().
		WithMode(librevm.ModeContainer).
		WithName(command.String(define.FlagSessionID)).
		WithCPUs(int(command.Int8(define.FlagCPUS))).
		WithMemory(command.Uint64(define.FlagMemoryInMB)).
		WithNetwork(command.String(define.FlagVNetworkType)).
		WithProxy(command.Bool(define.FlagUsingSystemProxy)).
		WithLogLevel(command.String(define.FlagLogLevel)).
		WithLogTo(command.String(define.FlagLogTo)).
		WithDisk(command.StringSlice(define.FlagRawDisk)...).
		WithMount(command.StringSlice(define.FlagMount)...).
		WithContainerDisk(command.String(define.FlagContainerDisk)).
		WithPodmanProxyAPIFile(command.String(define.FlagPodmanProxyAPIFile)).
		WithManageAPIFile(command.String(define.FlagManageAPIFile)).
		WithSSHKeyDir(command.String(define.FlagSSHKeyDir)).
		WithExportSSHKeyPrivateFile(command.String(define.FlagExportSSHKeyPrivateFile)).
		WithExportSSHKeyPublicFile(command.String(define.FlagExportSSHKeyPublicFile))

	// if legacy event reporter is set, use it
	if u := command.String(define.FlagOVMReportURL); u != "" {
		cfg.WithEventReporter(eventreporter.NewLegacyReporter(u, librevm.ModeContainer))
	}

	if u := command.String(define.FlagOVMContainerDiskVersion); u != "" {
		cfg.WithContainerDiskVersion(u)
	}

	// Apply init vmconfig preferences if present.
	if initCfg, err := librevm.LoadFile(vmConfigFilePath); err == nil {
		logrus.Infof("[apply-vmconfig] apply vmconfig prefer from: %q", vmConfigFilePath)
		cfg.MergeFrom(initCfg)
	}

	vm, err := librevm.New(cfg)
	if err != nil {
		return err
	}

	vm.Cancel = cancel

	defer vm.Close()

	return vm.RunDocker(ctx)
}
