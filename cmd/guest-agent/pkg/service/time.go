//go:build (linux || darwin) && (arm64 || amd64)

package service

import (
	"context"
	"guestAgent/pkg/supervisor"
	"time"
)

var servers = []string{
	"asia.pool.ntp.org",
}

func SyncRTCTime(ctx context.Context) error {
	args := []string{"ntpd", "-n"}
	for _, s := range servers {
		args = append(args, "-p", s)
	}

	sv := supervisor.New(supervisor.Config{
		Name:       "ntpd",
		Cmd:        BusyboxPath(),
		Args:       args,
		Stderr:     StderrWriter(),
		Stdout:     StderrWriter(),
		Restart:    true,
		RetryDelay: 5 * time.Second,

		MaxRunTimeout: 1 * time.Minute,
	})
	sv.Run(ctx)
	return nil
}
