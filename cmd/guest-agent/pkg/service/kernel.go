package service

import (
	"bufio"
	"context"
	"errors"
	"os"
	"os/exec"

	"github.com/sirupsen/logrus"
)

func StreamLogKernelMessage(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "dmesg", "-w")

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}

	if err := cmd.Start(); err != nil {
		return err
	}

	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			logrus.Info(scanner.Text())
		}
		if err := scanner.Err(); err != nil && !errors.Is(err, os.ErrClosed) {
			logrus.Warnf("dmesg stdout scanner error: %v", err)
		}
	}()

	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			logrus.Info(scanner.Text())
		}
		if err := scanner.Err(); err != nil && !errors.Is(err, os.ErrClosed) {
			logrus.Warnf("dmesg stderr scanner error: %v", err)
		}
	}()

	return cmd.Wait()
}
