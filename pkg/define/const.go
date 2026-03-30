package define

import (
	"time"
)

const (
	ContainerMode RunMode = iota
)

type RunMode int32

func (m RunMode) String() string {
	switch m {
	case ContainerMode:
		return "container"
	default:
		return "unknown"
	}
}

type VNetMode string

const (
	GVISOR VNetMode = "gvisor"
	TSI    VNetMode = "tsi"
)

const (
	BuiltinBusybox          = "/.bin/busybox"
	GuestAgentPathInGuest   = "/.bin/guest-agent"
	GuestHiddenBinDir       = "/.bin"
	VMConfigFilePathInGuest = "/vmconfig.json"
	HostDomainInGVPNet      = "host.containers.internal"
	VarDiskMountPoint       = "/var"
	DefaultRawDiskVersion   = "v1-0-0-ovm"

	DefaultGuestUser = "root"

	UnspecifiedAddress = "0.0.0.0"
	GuestIP            = "192.168.127.2"

	DefaultVSockPort = 25882

	LocalHost = "127.0.0.1"

	SSHLocalForwardListenPort = 6123

	ServiceNamePodman       = "podman"
	ServiceNameSSH          = "ssh"
	ServiceNameGuestNetwork = "guest-network"
)

const (
	RestAPIVMConfigURL = "/vmconfig"
)

const (
	EnvLogLevel = "LOG_LEVEL"

	DefaultTimeTicker   = 100 * time.Millisecond
	DefaultProbeTimeout = 60 * time.Second
)
