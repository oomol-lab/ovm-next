//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package librevm

import (
	"context"
	"fmt"
	"linuxvm/pkg/define"
	"linuxvm/pkg/interfaces"
	"linuxvm/pkg/libkrun"
	"linuxvm/pkg/service/lifecycle"
	sshsvc "linuxvm/pkg/service/ssh"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"syscall"
	"time"

	"al.essio.dev/pkg/shellescape"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
)

// VM represents a running (or ready-to-run) virtual machine.
// Close must always be called to release resources.
type VM struct {
	cfg *Config

	machine    *define.Machine
	provider   interfaces.VMMProvider
	svc        lifecycle.HostServices
	sessionDir string
	cleanup    func()
	Cancel     context.CancelFunc

	eventDispatcher eventDispatcher

	seq atomic.Uint64

	ooSSHAgent *sshsvc.OOSSHAgentService
}

// newProvider creates a libkrun Provider for the current platform.
func newProvider(mc *define.Machine) (interfaces.VMMProvider, error) {
	switch {
	case runtime.GOOS == "darwin" && runtime.GOARCH == "arm64":
	case runtime.GOOS == "linux" && (runtime.GOARCH == "arm64" || runtime.GOARCH == "amd64"):
	default:
		return nil, fmt.Errorf("unsupported platform: %s/%s", runtime.GOOS, runtime.GOARCH)
	}
	p := libkrun.NewProvider(mc)
	if err := p.Create(context.Background()); err != nil {
		return nil, fmt.Errorf("create libkrun VM: %w", err)
	}
	return p, nil
}

// Close 释放所有资源（文件锁、workspace 目录、event eventDispatcher）。
// 必须始终调用，即使 Run() 从未被调用。幂等。
func (vm *VM) Close() error {
	if vm.cleanup != nil {
		vm.cleanup()
	}
	vm.eventDispatcher.close()
	return nil
}

func buildTimeInfo() string {
	version := define.Version
	if version == "" {
		version = "unknown"
	}
	commit := define.CommitID
	if commit == "" {
		commit = "unknown"
	}
	buildDate := define.BuildDate
	if buildDate == "" {
		buildDate = "unknown"
	}

	return fmt.Sprintf("%s-%s-%s", version, commit, buildDate)
}

func New(cfg *Config) (*VM, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config must not be nil")
	}

	if err := setupLoggers(cfg.LogLevel, cfg.LogTo, cfg.SessionID); err != nil {
		return nil, fmt.Errorf("setup loggers: %w", err)
	}

	logrus.Infof("ovm build info: %s", buildTimeInfo())
	logrus.Infof("ovm cmdline: %q", os.Args)

	cfg, err := NormalizeConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("resolve defaults: %w", err)
	}

	vm := &VM{
		cfg:        cfg,
		sessionDir: getSessionDir(cfg.SessionID),
	}

	vm.eventDispatcher.addReporter(newLegacyReporter(cfg.ReportURL, cfg.RunMode))

	return vm, nil
}

// init acquires all heavyweight resources: workspace dirs, flock, SSH keys,
// disk images, libkrun provider, and host services. Called once at the
// start of Run(). On failure it cleans up after itself.
func (vm *VM) init(ctx context.Context) error {
	if err := vm.configureOOSSHAgentForward(); err != nil {
		return err
	}

	mc, cleanup, err := buildMachine(ctx, *vm.cfg, vm.sessionDir)
	if err != nil {
		return fmt.Errorf("build machine: %w", err)
	}

	if err := vm.createUserSymlinks(); err != nil {
		cleanup()
		return fmt.Errorf("create symlinks: %w", err)
	}

	vmp, err := newProvider(mc)
	if err != nil {
		cleanup()
		return fmt.Errorf("create vm provider: %w", err)
	}

	vm.machine = mc
	vm.provider = vmp
	vm.svc = lifecycle.NewHostServices(vmp)
	vm.cleanup = cleanup
	return nil
}

func (vm *VM) configureOOSSHAgentForward() error {
	localSocket := filepath.Join(vm.sessionDir, "socks", "oo-ssh-agent.sock")
	service := sshsvc.NewOOSSHAgentService(localSocket)
	if !service.Enabled() {
		return nil
	}

	merged, shouldStart, existingHostPath, err := mergeAutoForwardUnixRule(
		vm.cfg.ForwardUnix,
		service.GuestSocketPath(),
		service.LocalSocketPath(),
	)
	if err != nil {
		return fmt.Errorf("merge ssh agent forward unix rules: %w", err)
	}
	vm.cfg.ForwardUnix = merged

	if !shouldStart {
		logrus.Infof("skip builtin oo ssh agent: guest socket %q already forwarded to %q by user config", service.GuestSocketPath(), existingHostPath)
		return nil
	}

	vm.ooSSHAgent = service
	return nil
}

func mergeAutoForwardUnixRule(specs []string, guestPath, hostPath string) ([]string, bool, string, error) {
	forwardRules, err := parseForwardUnixRules(specs)
	if err != nil {
		return nil, false, "", err
	}

	if existingHostPath, exists := forwardRules[guestPath]; exists {
		if existingHostPath == hostPath {
			return specs, true, existingHostPath, nil
		}
		return specs, false, existingHostPath, nil
	}

	return append(specs, fmt.Sprintf("%s:%s", guestPath, hostPath)), true, "", nil
}

// createUserSymlinks links session-internal resources to user-specified paths.
// All actual files remain inside sessionDir; the symlinks are just a convenience
// bridge so external tools can find them at well-known locations.
func (vm *VM) createUserSymlinks() error {
	cfg := vm.cfg
	p := newMachinePathManager(vm.sessionDir)

	if cfg.PodmanProxyAPIFile != "" {
		if err := createSymlink(p.GetPodmanSocketFile(), cfg.PodmanProxyAPIFile); err != nil {
			return fmt.Errorf("podman proxy socket: %w", err)
		}
	}
	if cfg.ManageAPIFile != "" {
		if err := createSymlink(p.GetVMCtlSocketFile(), cfg.ManageAPIFile); err != nil {
			return fmt.Errorf("vmctl socket: %w", err)
		}
	}
	if cfg.SSHKeyPrivateFileSymbolLinks != "" {
		if err := createSymlink(p.GetSSHPrivateKeyFile(), cfg.SSHKeyPrivateFileSymbolLinks); err != nil {
			return fmt.Errorf("ssh private key: %w", err)
		}
	}
	if cfg.SSHKeyPublicFileSymbolLinks != "" {
		if err := createSymlink(p.GetSSHPrivateKeyFile()+".pub", cfg.SSHKeyPublicFileSymbolLinks); err != nil {
			return fmt.Errorf("ssh public key: %w", err)
		}
	}
	return nil
}

// RunDocker starts the VM in container mode and blocks until it exits.
func (vm *VM) RunDocker(ctx context.Context) error {
	if err := vm.init(ctx); err != nil {
		return err
	}

	vm.emit(EventVMStarting, "starting vm in container mode")

	g, ctx := errgroup.WithContext(ctx)

	vm.startOOSSHAgent(ctx)

	// Start ignition server
	g.Go(func() error {
		return vm.svc.StartIgnitionService(ctx)
	})

	// Start network stack
	g.Go(func() error {
		return vm.svc.StartNetworkStack(ctx)
	})

	// Start management API
	g.Go(func() error {
		return vm.svc.StartMachineManagementAPI(ctx)
	})

	// Start Podman proxy (wait for network first)
	g.Go(func() error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-vm.machine.Readiness.VNetHostReady:
			return vm.svc.StartPodmanProxy(ctx)
		}
	})

	// Monitor readiness events
	go vm.monitorReadinessEvents(ctx, true)

	// Monitor for shutdown signals
	go func() {
		vm.WaitAndShutdownMachine(ctx, vm.Cancel)
	}()

	// Wait for services to start
	svcErrCh := make(chan error, 1)
	go func() {
		svcErrCh <- g.Wait()
		close(svcErrCh)
	}()

	// Start VM when network is ready
	select {
	case <-ctx.Done():
		return <-svcErrCh
	case <-vm.machine.Readiness.VNetHostReady:
		logrus.Infof("boot virtual machine...")
		err := vm.svc.StartVirtualMachine(ctx)
		vm.Cancel()
		<-svcErrCh
		return err
	}
}

func (vm *VM) startOOSSHAgent(ctx context.Context) {
	if vm.ooSSHAgent == nil {
		return
	}

	go func() {
		if err := vm.ooSSHAgent.Run(ctx); err != nil {
			logrus.Warnf("oo ssh agent exited with error: %v", err)
		}
	}()
}

// monitorReadinessEvents monitors readiness channels and emits events.
// It runs until all expected events are received or context is cancelled.
func (vm *VM) monitorReadinessEvents(ctx context.Context, expectPodman bool) {
	podmanReady := !expectPodman // If not expecting, mark as already done
	sshReady := false
	networkReady := false

	for {
		if podmanReady && sshReady && networkReady {
			return
		}

		select {
		case <-ctx.Done():
			return
		case <-vm.machine.Readiness.PodmanReady:
			if expectPodman && !podmanReady {
				podmanReady = true
				vm.emit(EventPodmanReady, fmt.Sprintf("podman API proxy listening on %s", vm.machine.PodmanInfo.HostPodmanProxyAddr))
				logrus.Infof("podman API proxy ready on %s", vm.machine.PodmanInfo.HostPodmanProxyAddr)
			}
		case <-vm.machine.Readiness.SSHReady:
			if !sshReady {
				sshReady = true
				vm.emit(EventSSHReady, "ssh ready")
			}
		case <-vm.machine.Readiness.VNetHostReady:
			if !networkReady {
				networkReady = true
				vm.emit(EventNetworkReady, "host network ready")
			}
		}
	}
}

func (vm *VM) WaitAndShutdownMachine(ctx context.Context, cancel context.CancelFunc) {
	// Monitor parent process exit
	go func() {
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if os.Getppid() == 1 {
					logrus.Info("parent process exited, shutting down machine")

					// we force to exit the ovm after 30 seconds, this should never happen,
					// but we still log this
					go func() {
						<-time.After(30 * time.Second)
						logrus.Errorf("force to exit ovm, this should not happen")
						os.Exit(100)
					}()

					_ = vm.svc.StopVirtualMachine()
					cancel()
					return
				}
			}
		}
	}()

	// Monitor shutdown signals
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		defer signal.Stop(sigCh)

		select {
		case <-ctx.Done():
			return
		case <-sigCh:
			logrus.Info("received signal, shutting down")
			_ = vm.svc.StopVirtualMachine()
			cancel()
		}
	}()
}

// execIgnoreErr runs a command in the VM via SSH, logging but not returning errors.
func (vm *VM) execIgnoreErr(ctx context.Context, cmdline ...string) {
	client, err := sshsvc.MakeSSHClient(ctx, vm.machine)
	if err != nil {
		logrus.Warnf("ssh connect for %v: %v", cmdline, err)
		return
	}
	defer client.Close()

	if err := client.Run(ctx, shellescape.QuoteCommand(cmdline)); err != nil {
		logrus.Warnf("exec %v: %v", cmdline, err)
	}
}

func GenerateVMConfig(ctx context.Context, cfg *Config, path string) error {
	if err := setupLoggers(cfg.LogLevel, cfg.LogTo, cfg.SessionID); err != nil {
		return fmt.Errorf("setup loggers: %w", err)
	}

	vm := &VM{
		cfg: cfg,
	}

	vm.eventDispatcher.addReporter(newLegacyReporter(cfg.ReportURL, cfg.RunMode))

	defer vm.emit(EventExit, "")

	if err := cfg.WriteCfg(path); err != nil {
		vm.emit(EventError, err.Error())
		return err
	}

	vm.emit(EventSuccess, "")

	return nil
}
