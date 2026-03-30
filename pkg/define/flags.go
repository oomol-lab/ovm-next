package define

const (
	SubCommandRun  = "start"
	SubCommandStop = "attach"
	SubCommandInit = "init"

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
	FlagDataDisk                = "data-disk"
	FlagDataDiskVersion         = "data-disk-version"
	FlagPodmanProxyAPIFile      = "podman-api"
	FlagManageAPIFile           = "manage-api"
	FlagExportSSHKeyPrivateFile = "ssh-private-key"
	FlagExportSSHKeyPublicFile  = "ssh-public-key"
	FlagReportEvents            = "report-url"

	VarDataDiskUUID = "162cf68f-93c7-49ad-be53-45ed0e9fe42b"

	GuestLogConsolePort = "guest-logs"
	GuestTTYConsoleName = "default-tty-console"

	KrunStdinPortName  = "krun-stdin"
	KrunStdoutPortName = "krun-stdout"
	KrunStderrPortName = "krun-stderr"
)
