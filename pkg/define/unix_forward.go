package define

import (
	"fmt"
	"sort"
)

const UnixForwardVSockBasePort uint32 = 25900

type UnixSocketForwardRoute struct {
	GuestPath string
	HostPath  string
	VSockPort uint32
}

func BuildUnixSocketForwardRoutes(forwards map[string]string) ([]UnixSocketForwardRoute, error) {
	if len(forwards) == 0 {
		return nil, nil
	}

	keys := make([]string, 0, len(forwards))
	for guestPath := range forwards {
		keys = append(keys, guestPath)
	}
	sort.Strings(keys)

	routes := make([]UnixSocketForwardRoute, 0, len(keys))
	usedPorts := make(map[uint32]struct{}, len(keys))
	for i, guestPath := range keys {
		port, err := unixForwardVSockPortByIndex(i)
		if err != nil {
			return nil, err
		}
		if _, exists := usedPorts[port]; exists {
			return nil, fmt.Errorf("duplicate vsock port %d for guest path %q", port, guestPath)
		}
		usedPorts[port] = struct{}{}

		routes = append(routes, UnixSocketForwardRoute{
			GuestPath: guestPath,
			HostPath:  forwards[guestPath],
			VSockPort: port,
		})
	}

	return routes, nil
}

func unixForwardVSockPortByIndex(index int) (uint32, error) {
	if index < 0 {
		return 0, fmt.Errorf("invalid unix forward index %d", index)
	}

	if uint64(UnixForwardVSockBasePort)+uint64(index) > uint64(^uint32(0)) {
		return 0, fmt.Errorf("unix forward index %d overflows uint32 vsock port", index)
	}

	return UnixForwardVSockBasePort + uint32(index), nil
}
