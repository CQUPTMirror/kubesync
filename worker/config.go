package worker

import (
	"errors"
	"strconv"
	"strings"

	units "github.com/docker/go-units"
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

// UnmarshalJSON is the customized unmarshaler for MemBytes
func (m *MemBytes) UnmarshalText(s []byte) error {
	val, err := units.RAMInBytes(string(s))
	*m = MemBytes(val)
	return err
}

// LoadConfig loads configuration
func LoadConfig() (*Config, error) {
	var err error

	cfg := new(Config)

	cfg.Name = GetEnv("NAME", "")
	cfg.Provider = GetEnv("PROVIDER", "")
	cfg.Upstream = GetEnv("UPSTREAM", "")
	cfg.LogDir = GetEnv("LOG_DIR", "/var/log")
	cfg.MirrorDir = GetEnv("MIRROR_DIR", "/data")

	if cfg.Name == "" || cfg.Provider == "" || cfg.Upstream == "" {
		return cfg, errors.New("Failed to get mirror config")
	}

	cfg.Concurrent, err = strconv.Atoi(GetEnv("CONCURRENT", "3"))
	if err != nil {
		return cfg, err
	}

	cfg.Interval, err = strconv.Atoi(GetEnv("INTERVAL", "1440"))
	if err != nil {
		return cfg, err
	}

	cfg.Retry, err = strconv.Atoi(GetEnv("RETRY", "0"))
	if err != nil {
		return cfg, err
	}

	cfg.Timeout, err = strconv.Atoi(GetEnv("TIMEOUT", "0"))
	if err != nil {
		return cfg, err
	}

	cfg.Command = GetEnv("COMMAND", "")
	cfg.FailOnMatch = GetEnv("FAIL_ON_MATCH", "")
	cfg.SizePattern = GetEnv("SIZE_PATTERN", "")
	cfg.UseIPv6, _ = strconv.ParseBool(GetEnv("IPV6", ""))
	cfg.UseIPv4, _ = strconv.ParseBool(GetEnv("IPV4", ""))
	cfg.ExcludeFile = GetEnv("EXCLUDE_FILE", "")
	cfg.RsyncNoTimeo, _ = strconv.ParseBool(GetEnv("RSYNC_NO_TIMEOUT", ""))
	cfg.RsyncTimeout, _ = strconv.Atoi(GetEnv("RSYNC_TIMEOUT", ""))
	cfg.RsyncOptions = strings.Split(GetEnv("RSYNC_OPTIONS", ""), ";")
	cfg.RsyncOverride = strings.Split(GetEnv("RSYNC_OVERRIDE", ""), ";")
	cfg.Stage1Profile = GetEnv("STAGE1_PROFILE", "")

	cfg.ExecOnSuccess = strings.Split(GetEnv("EXEC_ON_SUCCESS", ""), ";")
	cfg.ExecOnFailure = strings.Split(GetEnv("EXEC_ON_FAILURE", ""), ";")

	cfg.APIBase = GetEnv("API", "")
	cfg.Addr = GetEnv("ADDR", ":6000")

	cfg.ZFSEnable, _ = strconv.ParseBool(GetEnv("ZFS", ""))
	cfg.Zpool = GetEnv("ZPOOL", "")

	cfg.BtrfsEnable, _ = strconv.ParseBool(GetEnv("BTRFS", ""))
	cfg.SnapshotPath = GetEnv("SNAPSHOT_PATH", "")

	return cfg, nil
}
