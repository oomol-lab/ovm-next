//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package librevm

import (
	"fmt"
	"path/filepath"
	"strings"
)

func parseForwardUnixRules(specs []string) (map[string]string, error) {
	if len(specs) == 0 {
		return nil, nil
	}

	rules := make(map[string]string, len(specs))
	for _, raw := range specs {
		spec := strings.TrimSpace(raw)
		if spec == "" {
			continue
		}

		guestPath, hostPath, found := strings.Cut(spec, ":")
		if !found {
			return nil, fmt.Errorf("invalid forward rule %q, expected guest-path:host-path", raw)
		}

		guestPath = strings.TrimSpace(guestPath)
		hostPath = strings.TrimSpace(hostPath)
		hostPath = strings.TrimPrefix(hostPath, "unix://")

		if guestPath == "" || hostPath == "" {
			return nil, fmt.Errorf("invalid forward rule %q, guest-path and host-path must not be empty", raw)
		}
		if !filepath.IsAbs(guestPath) {
			return nil, fmt.Errorf("invalid guest unix socket path %q: must be absolute", guestPath)
		}
		if !filepath.IsAbs(hostPath) {
			return nil, fmt.Errorf("invalid host unix socket path %q: must be absolute", hostPath)
		}

		if existing, ok := rules[guestPath]; ok {
			return nil, fmt.Errorf("duplicate guest unix socket path %q (conflicts with host path %q)", guestPath, existing)
		}
		rules[guestPath] = hostPath
	}

	return rules, nil
}
