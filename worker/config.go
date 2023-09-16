package worker

import (
	"errors"
	"github.com/docker/go-units"
)

// Config represents worker config options
type Config struct {
	Name       string `toml:"name"`
	Provider   string `toml:"provider"`
	Upstream   string `toml:"upstream"`
	LogDir     string `toml:"log_dir"`
	MirrorDir  string `toml:"mirror_dir"`
	Concurrent int    `toml:"concurrent"`
	Interval   int    `toml:"interval"`
	Retry      int    `toml:"retry"`
	Timeout    int    `toml:"timeout"`

	Command       string   `toml:"command"`
	FailOnMatch   string   `toml:"fail_on_match"`
	SizePattern   string   `toml:"size_pattern"`
	UseIPv6       bool     `toml:"use_ipv6"`
	UseIPv4       bool     `toml:"use_ipv4"`
	ExcludeFile   string   `toml:"exclude_file"`
	RsyncNoTimeo  bool     `toml:"rsync_no_timeout"`
	RsyncTimeout  int      `toml:"rsync_timeout"`
	RsyncOptions  []string `toml:"rsync_options"`
	RsyncOverride []string `toml:"rsync_override"`
	Stage1Profile string   `toml:"stage1_profile"`

	ExecOnSuccess []string `toml:"exec_on_success"`
	ExecOnFailure []string `toml:"exec_on_failure"`

	APIBase string `toml:"api_base"`
	Addr    string `toml:"listen_addr"`

	ZFSEnable bool   `toml:"zfs_enable"`
	Zpool     string `toml:"zpool"`

	BtrfsEnable  bool   `toml:"btrfs_enable"`
	SnapshotPath string `toml:"snapshot_path"`

	Verbose bool
	Debug   bool
}

type MemBytes int64

// Set sets the value of the MemBytes by passing a string
func (m *MemBytes) Set(value string) error {
	val, err := units.RAMInBytes(value)
	*m = MemBytes(val)
	return err
}

// Type returns the type
func (m *MemBytes) Type() string {
	return "bytes"
}

// Value returns the value in int64
func (m *MemBytes) Value() int64 {
	return int64(*m)
}

// UnmarshalText is the customized unmarshaler for MemBytes
func (m *MemBytes) UnmarshalText(s []byte) error {
	val, err := units.RAMInBytes(string(s))
	*m = MemBytes(val)
	return err
}

// LoadConfig loads configuration
func LoadConfig() (*Config, error) {
	cfg := new(Config)

	cfg.Verbose = GetBoolEnv("VERBOSE")
	cfg.Debug = GetBoolEnv("DEBUG")

	cfg.Name = GetStringEnv("NAME", "")
	cfg.Provider = GetStringEnv("PROVIDER", "")
	cfg.Upstream = GetStringEnv("UPSTREAM", "")
	cfg.LogDir = GetStringEnv("LOG_DIR", "/var/log")
	cfg.MirrorDir = GetStringEnv("MIRROR_DIR", "/data")

	if cfg.Name == "" || cfg.Provider == "" || cfg.Upstream == "" {
		return cfg, errors.New("failed to get mirror config")
	}

	cfg.Concurrent = GetIntEnv("CONCURRENT", 3)
	cfg.Interval = GetIntEnv("INTERVAL", 1440)
	cfg.Retry = GetIntEnv("RETRY", 0)
	cfg.Timeout = GetIntEnv("TIMEOUT", 0)

	cfg.Command = GetStringEnv("COMMAND", "")
	cfg.FailOnMatch = GetStringEnv("FAIL_ON_MATCH", "")
	cfg.SizePattern = GetStringEnv("SIZE_PATTERN", "")
	cfg.UseIPv6 = GetBoolEnv("IPV6")
	cfg.UseIPv4 = GetBoolEnv("IPV4")
	cfg.ExcludeFile = GetStringEnv("EXCLUDE_FILE", "")
	cfg.RsyncNoTimeo = GetBoolEnv("RSYNC_NO_TIMEOUT")
	cfg.RsyncTimeout = GetIntEnv("RSYNC_TIMEOUT", 0)
	cfg.RsyncOptions = GetListEnv("RSYNC_OPTIONS")
	cfg.RsyncOverride = GetListEnv("RSYNC_OVERRIDE")
	cfg.Stage1Profile = GetStringEnv("STAGE1_PROFILE", "")

	cfg.ExecOnSuccess = GetListEnv("EXEC_ON_SUCCESS")
	cfg.ExecOnFailure = GetListEnv("EXEC_ON_FAILURE")

	cfg.APIBase = GetStringEnv("API", "http://manager:3000")
	cfg.Addr = GetStringEnv("ADDR", ":6000")

	cfg.ZFSEnable = GetBoolEnv("ZFS")
	cfg.Zpool = GetStringEnv("ZPOOL", "")

	cfg.BtrfsEnable = GetBoolEnv("BTRFS")
	cfg.SnapshotPath = GetStringEnv("SNAPSHOT_PATH", "")

	return cfg, nil
}
