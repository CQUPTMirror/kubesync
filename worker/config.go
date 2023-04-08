package worker

import (
	"errors"
	"os"

	"github.com/BurntSushi/toml"
	units "github.com/docker/go-units"
)

type providerEnum uint8

const (
	provRsync providerEnum = iota
	provTwoStageRsync
	provCommand
)

func (p *providerEnum) UnmarshalText(text []byte) error {
	s := string(text)
	switch s {
	case `command`:
		*p = provCommand
	case `rsync`:
		*p = provRsync
	case `two-stage-rsync`:
		*p = provTwoStageRsync
	default:
		return errors.New("Invalid value to provierEnum")
	}
	return nil
}

// Config represents worker config options
type Config struct {
	Global        globalConfig        `toml:"global"`
	APIBase       string              `toml:"api_base"`
	Server        serverConfig        `toml:"server"`
	ZFS           zfsConfig           `toml:"zfs"`
	BtrfsSnapshot btrfsSnapshotConfig `toml:"btrfs_snapshot"`
	Mirror        mirrorConfig        `toml:"mirror"`
}

type globalConfig struct {
	Name       string `toml:"name"`
	Namespace  string `toml:"namespace"`
	LogDir     string `toml:"log_dir"`
	MirrorDir  string `toml:"mirror_dir"`
	Concurrent int    `toml:"concurrent"`
	Interval   int    `toml:"interval"`
	Retry      int    `toml:"retry"`
	Timeout    int    `toml:"timeout"`

	ExecOnSuccess []string `toml:"exec_on_success"`
	ExecOnFailure []string `toml:"exec_on_failure"`
}

type serverConfig struct {
	Addr string `toml:"listen_addr"`
	Port int    `toml:"listen_port"`
}

type zfsConfig struct {
	Enable bool   `toml:"enable"`
	Zpool  string `toml:"zpool"`
}

type btrfsSnapshotConfig struct {
	Enable       bool   `toml:"enable"`
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

type mirrorConfig struct {
	Name         string            `toml:"name"`
	Provider     providerEnum      `toml:"provider"`
	Upstream     string            `toml:"upstream"`
	Interval     int               `toml:"interval"`
	Retry        int               `toml:"retry"`
	Timeout      int               `toml:"timeout"`
	MirrorDir    string            `toml:"mirror_dir"`
	MirrorSubDir string            `toml:"mirror_subdir"`
	LogDir       string            `toml:"log_dir"`
	Env          map[string]string `toml:"env"`

	// These two options  the global options
	ExecOnSuccessExtra []string `toml:"exec_on_success_extra"`
	ExecOnFailureExtra []string `toml:"exec_on_failure_extra"`

	Command       string   `toml:"command"`
	FailOnMatch   string   `toml:"fail_on_match"`
	SizePattern   string   `toml:"size_pattern"`
	UseIPv6       bool     `toml:"use_ipv6"`
	UseIPv4       bool     `toml:"use_ipv4"`
	ExcludeFile   string   `toml:"exclude_file"`
	Username      string   `toml:"username"`
	Password      string   `toml:"password"`
	RsyncNoTimeo  bool     `toml:"rsync_no_timeout"`
	RsyncTimeout  int      `toml:"rsync_timeout"`
	RsyncOptions  []string `toml:"rsync_options"`
	RsyncOverride []string `toml:"rsync_override"`
	Stage1Profile string   `toml:"stage1_profile"`

	SnapshotPath string `toml:"snapshot_path"`
}

// LoadConfig loads configuration
func LoadConfig(cfgFile string) (*Config, error) {
	if _, err := os.Stat(cfgFile); err != nil {
		return nil, err
	}

	cfg := new(Config)
	if _, err := toml.DecodeFile(cfgFile, cfg); err != nil {
		logger.Errorf(err.Error())
		return nil, err
	}

	return cfg, nil
}
