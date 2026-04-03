package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"guestAgent/pkg/machine"
	"guestAgent/pkg/service"
	"io"
	"linuxvm/pkg/define"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v3"
	"golang.org/x/sync/errgroup"
)

// CmdlineExitNormal signals that the user command completed successfully.
// Used as a non-nil error to cancel the errgroup.
var CmdlineExitNormal = errors.New("command exited normally")

func setupLogger() {
	level := strings.ToLower(os.Getenv(define.EnvLogLevel))
	if level == "" {
		level = "info"
	}

	l, err := logrus.ParseLevel(level)
	if err != nil {
		panic(fmt.Errorf("invalid log level %q: %w", level, err))
	}

	logrus.SetLevel(l)
	logrus.SetFormatter(&logrus.TextFormatter{
		FullTimestamp:   true,
		TimestampFormat: "2006-01-02 15:04:05.000",
		ForceColors:     true,
	})
}

// setupGuestLogAndSignalPort opens the guest-logs port for logging and signal handling.
func setupGuestLogAndSignalPort(ctx context.Context) {
	f, err := openVirtioPort(define.GuestLogConsolePort, os.O_RDWR)
	if err != nil {
		logrus.Debugf("guest-log port not available: %v", err)
		return
	}

	logrus.SetOutput(io.MultiWriter(os.Stderr, f))
	service.SetStderrWriter(io.MultiWriter(os.Stderr, f))
	logrus.Infof("guest logs attached to virtio port %s", f.Name())

	go func() {
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			var msg struct {
				SignalName string `json:"signalName,omitempty"`
			}
			if json.Unmarshal(scanner.Bytes(), &msg) == nil {
				logrus.Infof("received signal: %s", msg.SignalName)

				// Parse signal name to syscall.Signal
				var sig syscall.Signal
				switch msg.SignalName {
				case "interrupt":
					sig = syscall.SIGINT
				case "terminated":
					sig = syscall.SIGTERM
				case "quit":
					sig = syscall.SIGQUIT
				default:
					logrus.Warnf("unknown signal name: %s", msg.SignalName)
					continue
				}

				// Forward signal to all child processes
				if err := syscall.Kill(-1, sig); err != nil {
					logrus.Errorf("failed to send %s to children: %v", msg.SignalName, err)
				}

				// Send signal to self to trigger WaitAndShutdown
				if err := syscall.Kill(os.Getpid(), sig); err != nil {
					logrus.Errorf("failed to send %s to self: %v", msg.SignalName, err)
				}
			}
		}
	}()
}

// openVirtioPort scans /sys/class/virtio-ports/*/name to find
// the device node for the given port name, then opens it with the specified flags.
func openVirtioPort(name string, flag int) (*os.File, error) {
	matches, err := filepath.Glob("/sys/class/virtio-ports/*/name")
	if err != nil {
		return nil, err
	}

	for _, path := range matches {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		if strings.TrimSpace(string(data)) == name {
			devName := filepath.Base(filepath.Dir(path))
			return os.OpenFile("/dev/"+devName, flag, 0)
		}
	}
	return nil, fmt.Errorf("virtio port %q not found", name)
}

func main() {
	app := cli.Command{
		Name:                      os.Args[0],
		Usage:                     "rootfs guest agent",
		UsageText:                 os.Args[0] + " [command] [flags]",
		Description:               "setup the guest environment, and run the command specified by the user.",
		Action:                    run,
		DisableSliceFlagSeparator: true,
	}

	if err := app.Run(context.Background(), os.Args); err != nil {
		if errors.Is(err, CmdlineExitNormal) {
			logrus.Infof("%v", CmdlineExitNormal)
			return
		}
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			logrus.Errorf("cmdline exit unexpected: %v", err)
			os.Exit(exitErr.ExitCode())
		}
		if errors.Is(err, exec.ErrNotFound) {
			logrus.Errorf("cmdline exit unexpected: %v", err)
			os.Exit(127)
		}

		logrus.Fatalf("cmdline exit unexpected: %v", err)
	}
}

func run(ctx context.Context, _ *cli.Command) error {
	setupLogger()

	if err := service.InitBinDir(); err != nil {
		return fmt.Errorf("init bin dir: %w", err)
	}

	vmc, err := service.GetVMConfig(ctx)
	if err != nil {
		return fmt.Errorf("get vm config: %w", err)
	}

	if err := service.MountAllPseudoMnt(ctx); err != nil {
		return fmt.Errorf("mount pseudo filesystems: %w", err)
	}

	// Now that /sys is available, setup guest-logs port for logging and signal handling
	setupGuestLogAndSignalPort(ctx)

	go func() {
		if err := service.StreamLogKernelMessage(ctx); err != nil {
			// log as INFO, because it's not a critical error/waring
			logrus.Info("[dmesg] log kernel message stop: %v", err)
		}
	}()

	if err := service.MountBlockDevices(ctx, vmc); err != nil {
		return fmt.Errorf("mount block devices: %w", err)
	}

	if err := service.MountVirtiofs(ctx, vmc); err != nil {
		return fmt.Errorf("mount virtiofs: %w", err)
	}
	go func() {
		service.WaitAndShutdown()
	}()

	return dockerEngineMode(ctx, vmc)
}

func dockerEngineMode(ctx context.Context, vmc *define.Machine) error {
	// Configure network before starting services — it's a prerequisite,
	// not a parallel task. If DHCP fails (e.g. eth0 not yet created by VMM),
	// we don't want to cancel already-running services.
	if err := service.ConfigureNetwork(ctx, (*machine.Machine)(vmc).GetVirtualNetworkType()); err != nil {
		return fmt.Errorf("configure network: %w", err)
	}

	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		logrus.Info("starting container engine")
		return service.StartGuestPodmanService(ctx, vmc)
	})

	g.Go(func() error {
		logrus.Info("starting ssh server")
		return service.StartGuestSSHServer(ctx, vmc)
	})

	g.Go(func() error {
		logrus.Info("starting syncing time loop")
		return service.SyncRTCTime(ctx)
	})

	// Run readiness probes outside the errgroup. Probe failures are logged
	// internally and do not affect service lifecycle.
	go func() {
		_ = machine.WaitGuestServiceReady(ctx, vmc) // short time function
	}()

	errChan := make(chan error, 1)
	go func() {
		errChan <- g.Wait()
		close(errChan)
	}()

	select {
	case <-ctx.Done():
		return context.Cause(ctx)
	case err := <-errChan:
		return err
	}
}
