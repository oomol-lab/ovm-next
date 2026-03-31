//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package librevm

import (
	"encoding/json"
	"fmt"
	"io"
	"linuxvm/pkg/define"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/google/uuid"
	"github.com/shirou/gopsutil/v4/mem"
)

// RunMode selects the VM run mode.
type RunMode string

const (
	// ModeContainer boots the VM with the built-in container runtime (Podman).
	ModeContainer RunMode = "docker"
	ModeCfgGen    RunMode = "cfggen"
)

func (m RunMode) IsValid() bool {
	switch m {
	case ModeContainer:
		return true
	default:
		return false
	}
}

type Config struct {
	RunMode   RunMode `toml:"runMode,omitempty" json:"runMode,omitempty"`
	SessionID string  `toml:"sessionID,omitempty" json:"sessionID,omitempty"` // session name
	CPUs      int     `toml:"cpus,omitempty"      json:"cpus,omitempty"`      // 0 → host CPU count
	MemoryMB  uint64  `toml:"memory_mb,omitempty" json:"memoryMB,omitempty"`  // 0 → host total RAM

	// Command specifies the program to run inside the VM (rootfs mode only).
	Command []string `toml:"command,omitempty"  json:"command,omitempty"`
	WorkDir string   `toml:"workdir,omitempty"  json:"workdir,omitempty"`
	Env     []string `toml:"env,omitempty"      json:"env,omitempty"`

	// RawDisk describes a host raw disk image and where it should mount inside the guest.
	// If Mnt is empty, the guest defaults to /mnt/<UUID>.
	VarDisk       RawDisk   `toml:"varDisk,omitempty"       json:"varDisk,omitempty"`
	ExternalDisks []RawDisk `toml:"externalDisks,omitempty" json:"externalDisks,omitempty"`

	Network                      string          `toml:"network,omitempty"         json:"network,omitempty"` // "gvisor" | "tsi"
	Mounts                       []string        `toml:"mounts,omitempty"          json:"mounts,omitempty"`  // "/host:/guest[,ro]"
	PodmanProxyAPIFile           string          `toml:"podman_proxy_api_file,omitempty"   json:"podmanProxyAPIFile,omitempty"`
	ManageAPIFile                string          `toml:"manage_api_file,omitempty"         json:"manageAPIFile,omitempty"`
	SSHKeyPrivateFileSymbolLinks string          `toml:"ssh_key_private_file_symbol_links,omitempty" json:"SSHKeyPrivateFileSymbolLinks,omitempty"`
	SSHKeyPublicFileSymbolLinks  string          `toml:"ssh_key_public_file_symbol_links,omitempty" json:"SSHKeyPublicFileSymbolLinks,omitempty"`
	Proxy                        bool            `toml:"proxy,omitempty"           json:"proxy,omitempty"`
	LogLevel                     string          `toml:"log_level,omitempty"       json:"logLevel,omitempty"` // default "info"
	LogTo                        string          `toml:"log_to,omitempty"          json:"logTo,omitempty"`
	Reporters                    []EventReporter `toml:"-" json:"-"`
}

type RawDisk struct {
	RawDiskPath string `toml:"raw_disk_path,omitempty" json:"rawDiskPath,omitempty"`
	UUID        string `toml:"uuid,omitempty"          json:"uuid,omitempty"`
	Mnt         string `toml:"mnt,omitempty"           json:"mnt,omitempty"`
	Version     string `toml:"version,omitempty"       json:"version,omitempty"`
}

// DefaultConfig returns a Config with sensible defaults pre-filled.
// Zero-value resource fields (CPUs, MemoryMB) are resolved at VM creation time.
func DefaultConfig(id string) *Config {
	return &Config{
		SessionID: id,
		Network:   "gvisor",
		LogLevel:  "info",
		WorkDir:   "/",
		VarDisk: RawDisk{
			RawDiskPath: getDefaultVarDiskPath(id),
			UUID:        define.VarDataDiskUUID,
			Mnt:         define.VarDiskMountPoint,
			Version:     define.DefaultRawDiskVersion,
		},
	}
}

// --- Chain (fluent) methods ------------------------------------------------

func (c *Config) WithMode(m RunMode) *Config {
	if m != "" {
		c.RunMode = m
	}
	return c
}
func (c *Config) WithName(name string) *Config {
	if name != "" {
		c.SessionID = name
	}
	return c
}
func (c *Config) WithCPUs(n int) *Config {
	if n > 0 {
		c.CPUs = n
	}
	return c
}
func (c *Config) WithMemory(mb uint64) *Config {
	if mb > 0 {
		c.MemoryMB = mb
	}
	return c
}
func (c *Config) WithWorkDir(dir string) *Config {
	if dir != "" {
		c.WorkDir = dir
	}
	return c
}

func (c *Config) WithNetwork(mode string) *Config {
	if mode != "" {
		c.Network = mode
	}
	return c
}

func (c *Config) WithRawDisk(disks ...RawDisk) *Config {
	c.ExternalDisks = append(c.ExternalDisks, disks...)
	return c
}

func (c *Config) WithVarDataDisk(spec string) *Config {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return c
	}

	parts := strings.Split(spec, ",")
	path := strings.TrimSpace(parts[0])
	if path == "" {
		return c
	}

	version := define.DefaultRawDiskVersion
	for _, part := range parts[1:] {
		part = strings.TrimSpace(part)
		key, val, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(key), "version") {
			if strings.TrimSpace(val) != "" {
				version = strings.TrimSpace(val)
			}
		}
	}

	c.VarDisk = RawDisk{
		RawDiskPath: path,
		UUID:        define.VarDataDiskUUID,
		Mnt:         define.VarDiskMountPoint,
		Version:     version,
	}

	return c
}

func (c *Config) WithPodmanProxyAPIFile(path string) *Config {
	if path != "" {
		c.PodmanProxyAPIFile = path
	}
	return c
}
func (c *Config) WithManageAPIFile(path string) *Config {
	if path != "" {
		c.ManageAPIFile = path
	}
	return c
}
func (c *Config) WithExportSSHKeyPrivateFile(path string) *Config {
	if path != "" {
		c.SSHKeyPrivateFileSymbolLinks = path
	}
	return c
}
func (c *Config) WithExportSSHKeyPublicFile(path string) *Config {
	if path != "" {
		c.SSHKeyPublicFileSymbolLinks = path
	}
	return c
}
func (c *Config) WithEventReporter(reporters ...EventReporter) *Config {
	for _, r := range reporters {
		if r != nil {
			c.Reporters = append(c.Reporters, r)
		}
	}
	return c
}
func (c *Config) WithProxy(enable bool) *Config { c.Proxy = enable; return c }
func (c *Config) WithLogLevel(level string) *Config {
	if level != "" {
		c.LogLevel = level
	}
	return c
}
func (c *Config) WithLogTo(path string) *Config {
	if path != "" {
		c.LogTo = path
	}
	return c
}

func (c *Config) WithCommand(bin string, args ...string) *Config {
	if bin != "" {
		c.Command = append([]string{bin}, args...)
	}
	return c
}

func (c *Config) WithEnv(kvs ...string) *Config {
	if len(kvs) > 0 {
		c.Env = append(c.Env, kvs...)
	}
	return c
}

func (c *Config) WithMount(specs ...string) *Config {
	if len(specs) > 0 {
		c.Mounts = append(c.Mounts, specs...)
	}
	return c
}

func (c *Config) WithDisk(specs ...string) *Config {
	if len(specs) == 0 {
		return c
	}

	for _, spec := range specs {
		spec = strings.TrimSpace(spec)
		if spec == "" {
			continue
		}

		parts := strings.Split(spec, ",")
		raw := RawDisk{
			RawDiskPath: strings.TrimSpace(parts[0]),
		}

		for _, part := range parts[1:] {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}

			key, val, ok := strings.Cut(part, "=")
			if !ok {
				// Backward compatible: --raw-disk <path>,<uuid>
				if raw.UUID == "" {
					raw.UUID = part
				}
				continue
			}

			key = strings.ToLower(strings.TrimSpace(key))
			val = strings.TrimSpace(val)
			switch key {
			case "uuid":
				raw.UUID = val
			case "version":
				raw.Version = val
			case "mnt":
				raw.Mnt = val
			}
		}

		if raw.Version == "" {
			raw.Version = define.DefaultRawDiskVersion
		}
		if raw.UUID == "" {
			raw.UUID = uuid.NewString()
		}

		c.WithRawDisk(raw)
	}

	return c
}

// WriteCfg marshals cfg as JSON and writes it to path.
func (c *Config) WriteCfg(path string) error {
	data, err := json.Marshal(c)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("write config file: %w", err)
	}
	return nil
}

// MergeFrom applies non-zero preference fields from other onto c.
// Only fields that "init" is expected to set are merged; runtime-only
// fields (RunMode, SessionID, event report URLs, etc.) are intentionally skipped.
func (c *Config) MergeFrom(other *Config) {
	if other == nil {
		return
	}

	if other.SessionID != "" {
		c.SessionID = other.SessionID
	}

	if other.VarDisk.RawDiskPath != "" {
		c.VarDisk = other.VarDisk
	}
	if len(other.ExternalDisks) > 0 {
		c.ExternalDisks = other.ExternalDisks
	}

	if other.CPUs > 0 {
		c.CPUs = other.CPUs
	}
	if other.MemoryMB > 0 {
		c.MemoryMB = other.MemoryMB
	}
	if other.Network != "" {
		c.Network = other.Network
	}

	if len(other.Mounts) > 0 {
		c.Mounts = append(c.Mounts, other.Mounts...)
	}

	if other.PodmanProxyAPIFile != "" {
		c.PodmanProxyAPIFile = other.PodmanProxyAPIFile
	}

	if other.ManageAPIFile != "" {
		c.ManageAPIFile = other.ManageAPIFile
	}

	if other.SSHKeyPrivateFileSymbolLinks != "" {
		c.SSHKeyPrivateFileSymbolLinks = other.SSHKeyPrivateFileSymbolLinks
	}

	if other.SSHKeyPublicFileSymbolLinks != "" {
		c.SSHKeyPublicFileSymbolLinks = other.SSHKeyPublicFileSymbolLinks
	}

	if other.LogTo != "" {
		c.LogTo = other.LogTo
	}
}

// LoadFile reads a Config from path. The format is detected by extension:
// .toml for TOML, .json for JSON. Any other extension returns an error.
func LoadFile(path string) (*Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".toml":
		return Load(f)
	case ".json":
		return loadJSON(f)
	default:
		return nil, fmt.Errorf("unsupported config format %q (use .toml or .json)", ext)
	}
}

// Load reads a TOML-encoded Config from r.
func Load(r io.Reader) (*Config, error) {
	var cfg Config
	if _, err := toml.NewDecoder(r).Decode(&cfg); err != nil {
		return nil, fmt.Errorf("decode toml config: %w", err)
	}
	return &cfg, nil
}

func loadJSON(r io.Reader) (*Config, error) {
	var cfg Config
	if err := json.NewDecoder(r).Decode(&cfg); err != nil {
		return nil, fmt.Errorf("decode json config: %w", err)
	}
	return &cfg, nil
}

// NormalizeConfig returns a copy of cfg with defaults resolved.
func NormalizeConfig(cfg *Config) (*Config, error) {
	if cfg.CPUs <= 0 {
		cfg.CPUs = runtime.NumCPU()
	}

	if cfg.MemoryMB == 0 {
		m, err := mem.VirtualMemory()
		if err != nil {
			return nil, fmt.Errorf("detect host memory: %w", err)
		}
		cfg.MemoryMB = m.Total / 1024 / 1024
	}

	if cfg.Network == "" {
		cfg.Network = "gvisor"
	}

	if cfg.LogLevel == "" {
		cfg.LogLevel = "info"
	}

	if cfg.WorkDir == "" {
		cfg.WorkDir = "/"
	}

	if err := validateConfig(cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

func validateConfig(cfg *Config) error {
	if cfg.SessionID == "" {
		return fmt.Errorf("session name must not be empty, flag --id is required")
	}

	if !cfg.RunMode.IsValid() {
		return fmt.Errorf("invalid run mode %q", cfg.RunMode)
	}

	if cfg.MemoryMB < 512 {
		return fmt.Errorf("memory must be at least 512 MB, got %d", cfg.MemoryMB)
	}

	if cfg.CPUs < 1 {
		return fmt.Errorf("cpus must be at least 1, got %d", cfg.CPUs)
	}
	if cfg.CPUs > 255 {
		return fmt.Errorf("cpus must be at most 255 (libkrun uint8_t limit), got %d", cfg.CPUs)
	}

	switch cfg.Network {
	case "gvisor", "tsi":
		// ok
	default:
		return fmt.Errorf("network must be \"gvisor\" or \"tsi\", got %q", cfg.Network)
	}

	if cfg.VarDisk.RawDiskPath == "" {
		return fmt.Errorf("var disk path is required")
	}

	if cfg.VarDisk.Mnt != define.VarDiskMountPoint || cfg.VarDisk.UUID != define.VarDataDiskUUID {
		return fmt.Errorf("var disk must use mnt=%q and uuid=%q", define.VarDiskMountPoint, define.VarDataDiskUUID)
	}

	return nil
}
