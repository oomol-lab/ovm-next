package librevm

import (
	"context"
	"fmt"
	"linuxvm/pkg/network"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

type legacyReporter struct {
	client  *network.Client
	runMode string // raw runMode, mapped by evtProxy at report time
}

// newLegacyReporter creates a legacy eventReporter that sends GET /notify requests.
// Returns nil if the endpoint is invalid.
func newLegacyReporter(endpoint string, runMode RunMode) eventReporter {
	if endpoint == "" {
		return nil
	}

	client := newClient(endpoint)
	if client == nil {
		return nil
	}
	return &legacyReporter{client: client, runMode: string(runMode)}
}

func (r *legacyReporter) Report(evt Event) {
	stage, kind := r.evtProxy(r.runMode, evt.Kind)
	value := ""
	if kind == EventError {
		value = evt.Message
	}

	req := r.client.Get("/notify").
		Query("stage", stage).
		Query("name", string(kind)).
		Query("value", value)
	resp, err := req.Do(context.Background()) //nolint:bodyclose
	if err != nil {
		logrus.Warnf("legacy event sink: publish %s failed: %v", evt.Kind, err)
		return
	}
	network.CloseResponse(resp)
}

func (r *legacyReporter) Close() {
	if err := r.client.Close(); err != nil {
		logrus.Warnf("legacy event sink: close failed: %v", err)
	}
}

// for compatibility ovm-js
//
// runMode mapping:
//
//	cfggen -> init,
//	container -> start
//
// event kind mapping:
//
//	stopped -> Exit,
//	podmanReady -> Ready,
//	error -> Error,
//	success -> Success,
//	exit -> Exit
func (r *legacyReporter) evtProxy(runMode string, kind EventKind) (string, EventKind) {
	switch runMode {
	case string(ModeCfgGen):
		runMode = "init"
	case string(ModeContainer):
		runMode = "start"
	}
	switch kind {
	case EventStopped:
		kind = "Exit"
	case EventPodmanReady:
		kind = "Ready"
	case EventError:
		kind = "Error"
	case EventSuccess:
		kind = "Success"
	case EventExit:
		kind = "Exit"
	}
	return runMode, kind
}

func newClient(endpoint string) *network.Client {
	switch {
	case strings.HasPrefix(endpoint, "unix://") || strings.HasPrefix(endpoint, "unixgram://"):
		addr, err := network.ParseUnixAddr(endpoint)
		if err != nil {
			logrus.Warnf("event sink: invalid unix endpoint %q: %v", endpoint, err)
			return nil
		}
		return network.NewUnixClient(addr.Path, network.WithTimeout(1*time.Second))
	case strings.HasPrefix(endpoint, "tcp://"):
		addr, err := network.ParseTcpAddr(endpoint)
		if err != nil {
			logrus.Warnf("event sink: invalid tcp endpoint %q: %v", endpoint, err)
			return nil
		}
		hostPort := fmt.Sprintf("%s:%d", addr.Host, addr.Port)
		return network.NewTCPClient(hostPort, network.WithTimeout(1*time.Second))
	default:
		logrus.Warnf("event sink: unsupported endpoint scheme %q", endpoint)
		return nil
	}
}
