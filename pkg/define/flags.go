package define

const (
	SubCommandRun  = "start"
	SubCommandStop = "attach"

	FlagLogLevel                = "log-level"
	FlagLogTo                   = "log-to"
	FlagCPUS                    = "cpus"
	FlagRawDisk                 = "raw-disk"
	FlagMount                   = "mount"
	FlagUsingSystemProxy        = "system-proxy"
	FlagMemoryInMB              = "memory"
	FlagPTY                     = "pty"
	FlagEnvs                    = "envs"
	FlagVNetworkType            = "network"
	FlagSessionID               = "id"
	FlagContainerDisk           = "container-disk"
	FlagPodmanProxyAPIFile      = "podman-proxy-api-file"
	FlagManageAPIFile           = "manage-api-file"
	FlagSSHKeyDir               = "ssh-key-dir"
	FlagExportSSHKeyPrivateFile = "export-ssh-private-key"
	FlagExportSSHKeyPublicFile  = "export-ssh-public-key"
	FlagReportEvents            = "report-events-to"

	ContainerDiskUUID = "162cf68f-93c7-49ad-be53-45ed0e9fe42b"

	GuestLogConsolePort = "guest-logs"
	GuestTTYConsoleName = "default-tty-console"

	KrunStdinPortName  = "krun-stdin"
	KrunStdoutPortName = "krun-stdout"
	KrunStderrPortName = "krun-stderr"
)

const (
	FlagOVMBoot                 = "boot"
	FlagOVMBootVersion          = "boot-version"
	FlagOVMContainerDiskVersion = "data-version"
	FlagOVMPPID                 = "ppid"
	FlagOVMVolume               = "volume"
	FlagOVMName                 = "name"
	FlagOVMWorkspace            = "workspace"
	FlagOVMReportURL            = "report-url"

	DefaultOVMSessionID = "oomol-studio-19452"
	OVMSourceDiskUUID   = "44f7d1c0-122c-4402-a20e-c1166cbbad6d"
)
