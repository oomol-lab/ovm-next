package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"linuxvm/pkg/define"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/mdlayher/vsock"
	"github.com/sirupsen/logrus"
)

type unixForwardRule struct {
	guestPath string
	hostPath  string
	vsockPort uint32
}

func StartUnixForwarders(ctx context.Context, vmc *define.Machine) {
	if vmc == nil || len(vmc.UnixSocketForwards) == 0 {
		return
	}

	routes, err := define.BuildUnixSocketForwardRoutes(vmc.UnixSocketForwards)
	if err != nil {
		logrus.Errorf("normalize unix forward rules failed: %v", err)
		return
	}

	forwardRules := buildUnixForwardRules(routes)
	for _, rule := range forwardRules {
		if err := StartUnixForwarder(ctx, rule); err != nil {
			// Non-fatal by design: one bad rule should not block VM boot.
			logrus.Errorf("start unix forwarder %s -> %s failed: %v", rule.guestPath, rule.hostPath, err)
		}
	}
}

func buildUnixForwardRules(routes []define.UnixSocketForwardRoute) []unixForwardRule {
	forwardRules := make([]unixForwardRule, 0, len(routes))
	for _, route := range routes {
		forwardRules = append(forwardRules, unixForwardRule{
			guestPath: route.GuestPath,
			hostPath:  route.HostPath,
			vsockPort: route.VSockPort,
		})
	}

	return forwardRules
}

func StartUnixForwarder(ctx context.Context, rule unixForwardRule) error {
	if rule.guestPath == "" {
		return errors.New("guest unix socket path is empty")
	}
	if rule.hostPath == "" {
		return errors.New("host unix socket path is empty")
	}

	if err := os.Remove(rule.guestPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove existing unix socket %s: %w", rule.guestPath, err)
	}

	if err := os.MkdirAll(filepath.Dir(rule.guestPath), 0755); err != nil {
		return fmt.Errorf("create unix socket directory %s: %w", filepath.Dir(rule.guestPath), err)
	}

	ln, err := net.Listen("unix", rule.guestPath)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", rule.guestPath, err)
	}

	logrus.Infof("unix forwarder: listen %s -> vsock://2:%d (%s)", rule.guestPath, rule.vsockPort, rule.hostPath)

	go func() {
		<-ctx.Done()
		_ = ln.Close()
		_ = os.Remove(rule.guestPath)
	}()

	go serveUnixForwarder(ctx, ln, rule)
	return nil
}

func serveUnixForwarder(ctx context.Context, ln net.Listener, rule unixForwardRule) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				logrus.Infof("unix forwarder: %s closed", rule.guestPath)
				return
			}

			select {
			case <-ctx.Done():
				return
			default:
			}

			logrus.Warnf("unix forwarder accept error on %s: %v", rule.guestPath, err)
			time.Sleep(100 * time.Millisecond)
			continue
		}

		go handleUnixForwardConn(ctx, conn, rule)
	}
}

func handleUnixForwardConn(ctx context.Context, guestConn net.Conn, rule unixForwardRule) {
	hostConn, err := vsock.Dial(2, rule.vsockPort, nil)
	if err != nil {
		logrus.Warnf("unix forwarder dial vsock://2:%d for %s failed: %v", rule.vsockPort, rule.hostPath, err)
		_ = guestConn.Close()
		return
	}

	relayBidirectional(ctx, guestConn, hostConn)
}

func relayBidirectional(ctx context.Context, left, right net.Conn) {
	var wg sync.WaitGroup
	wg.Add(2)

	copyAndShutdown := func(dst, src net.Conn) {
		defer wg.Done()
		_, _ = io.Copy(dst, src)
		if closer, ok := dst.(interface{ CloseWrite() error }); ok {
			_ = closer.CloseWrite()
		}
	}

	go copyAndShutdown(right, left)
	go copyAndShutdown(left, right)

	done := make(chan struct{})
	go func() {
		defer close(done)
		wg.Wait()
	}()

	select {
	case <-ctx.Done():
	case <-done:
	}

	_ = left.Close()
	_ = right.Close()
}
