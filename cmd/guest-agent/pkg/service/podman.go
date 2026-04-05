package service

import (
	"context"
	_ "embed"
	"fmt"
	"guestAgent/pkg/supervisor"
	"linuxvm/pkg/define"
	"os"
	"path/filepath"
	"time"

	"github.com/sirupsen/logrus"
)

//go:embed init_d_podman.sh
var podmanInitDSh string

// generateCompatScripts 会创建 /etc/init.d/podman，这个 shell script 文件什么都不做，主要是兼容
// https://github.com/oomol/oomol-studio-code/blob/96b3a492f29f581319cfe13c21d5dce400a120ee/oomol-studio-main/desktop/container-server/image/sh/load_images.sh#L17
func generateCompatScripts() error {
	scriptFile := "/etc/init.d/podman"
	logrus.Infof("create podman init rc file, but this rc file do nothing just for compatibility")
	if err := os.MkdirAll(filepath.Dir(scriptFile), 0755); err != nil {
		return err
	}

	err := os.WriteFile(scriptFile, []byte(podmanInitDSh), 0755)
	if err != nil {
		return fmt.Errorf("failed to create podman init rc file: %s", err)
	}

	return os.WriteFile("/opt/ssh_auth", []byte{}, 0755)
}

func StartGuestPodmanService(ctx context.Context, vmc *define.Machine) error {
	if err := generateCompatScripts(); err != nil {
		return err
	}

	addr := "tcp://" + vmc.PodmanInfo.GuestPodmanAPIListenAddr //nolint:nosprintfhostport

	s := supervisor.New(supervisor.Config{
		Cmd: "podman",
		Args: []string{
			"--log-level", logrus.GetLevel().String(), "system", "service",
			"--time=0", addr,
		},
		Stdout:      StderrWriter(),
		Stderr:      StderrWriter(),
		Restart:     true,
		RetryDelay:  500 * time.Millisecond,
		StopTimeout: 5 * time.Second,
	})

	s.Run(ctx)
	return nil
}
