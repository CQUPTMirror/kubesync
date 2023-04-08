package worker

import (
	"bytes"
	"errors"
	"html/template"
	"path/filepath"
	"time"
)

// mirror provider is the wrapper of mirror jobs

const (
	_WorkingDirKey = "working_dir"
	_LogDirKey     = "log_dir"
	_LogFileKey    = "log_file"
)

// A mirrorProvider instance
type mirrorProvider interface {
	// name
	Name() string
	Upstream() string

	// Start then Wait
	Run(started chan empty) error
	// Start the job
	Start() error
	// Wait job to finish
	Wait() error
	// terminate mirror job
	Terminate() error
	// job hooks
	IsRunning() bool
	// ZFS
	ZFS() *zfsHook

	AddHook(hook jobHook)
	Hooks() []jobHook

	Interval() time.Duration
	Retry() int
	Timeout() time.Duration

	WorkingDir() string
	LogDir() string
	LogFile() string
	DataSize() string

	// enter context
	EnterContext() *Context
	// exit context
	ExitContext() *Context
	// return context
	Context() *Context
}

// newProvider creates a mirrorProvider instance
// using the global cfg
func newMirrorProvider(cfg *Config) mirrorProvider {

	formatLogDir := func(logDir string) string {
		tmpl, err := template.New("logDirTmpl-" + cfg.Name).Parse(logDir)
		if err != nil {
			panic(err)
		}
		var formatedLogDir bytes.Buffer
		tmpl.Execute(&formatedLogDir, cfg)
		return formatedLogDir.String()
	}

	mirrorDir := filepath.Join(cfg.MirrorDir, cfg.Name)
	logDir := formatLogDir(cfg.LogDir)

	var provider mirrorProvider

	switch cfg.Provider {
	case "command":
		pc := cmdConfig{
			name:        cfg.Name,
			upstreamURL: cfg.Upstream,
			command:     cfg.Command,
			workingDir:  mirrorDir,
			failOnMatch: cfg.FailOnMatch,
			sizePattern: cfg.SizePattern,
			logDir:      logDir,
			logFile:     filepath.Join(logDir, "latest.log"),
			interval:    time.Duration(cfg.Interval) * time.Minute,
			retry:       cfg.Retry,
			timeout:     time.Duration(cfg.Timeout) * time.Second,
		}
		p, err := newCmdProvider(pc)
		if err != nil {
			panic(err)
		}
		provider = p
	case "rsync":
		rc := rsyncConfig{
			name:              cfg.Name,
			upstreamURL:       cfg.Upstream,
			rsyncCmd:          cfg.Command,
			excludeFile:       cfg.ExcludeFile,
			extraOptions:      cfg.RsyncOptions,
			rsyncNeverTimeout: cfg.RsyncNoTimeo,
			rsyncTimeoutValue: cfg.RsyncTimeout,
			overriddenOptions: cfg.RsyncOverride,
			workingDir:        mirrorDir,
			logDir:            logDir,
			logFile:           filepath.Join(logDir, "latest.log"),
			useIPv6:           cfg.UseIPv6,
			useIPv4:           cfg.UseIPv4,
			interval:          time.Duration(cfg.Interval) * time.Minute,
			retry:             cfg.Retry,
			timeout:           time.Duration(cfg.Timeout) * time.Second,
		}
		p, err := newRsyncProvider(rc)
		if err != nil {
			panic(err)
		}
		provider = p
	case "two-stage-rsync":
		rc := twoStageRsyncConfig{
			name:              cfg.Name,
			stage1Profile:     cfg.Stage1Profile,
			upstreamURL:       cfg.Upstream,
			rsyncCmd:          cfg.Command,
			excludeFile:       cfg.ExcludeFile,
			extraOptions:      cfg.RsyncOptions,
			rsyncNeverTimeout: cfg.RsyncNoTimeo,
			rsyncTimeoutValue: cfg.RsyncTimeout,
			workingDir:        mirrorDir,
			logDir:            logDir,
			logFile:           filepath.Join(logDir, "latest.log"),
			useIPv6:           cfg.UseIPv6,
			useIPv4:           cfg.UseIPv4,
			interval:          time.Duration(cfg.Interval) * time.Minute,
			retry:             cfg.Retry,
			timeout:           time.Duration(cfg.Timeout) * time.Second,
		}
		p, err := newTwoStageRsyncProvider(rc)
		if err != nil {
			panic(err)
		}
		provider = p
	default:
		panic(errors.New("Invalid mirror provider"))
	}

	// Add Logging Hook
	provider.AddHook(newLogLimiter(provider))

	// Add ZFS Hook
	if cfg.ZFSEnable {
		provider.AddHook(newZfsHook(provider, cfg.Zpool))
	}

	// Add Btrfs Snapshot Hook
	if cfg.BtrfsEnable {
		provider.AddHook(newBtrfsSnapshotHook(provider, cfg.SnapshotPath))
	}

	addHookFromCmdList := func(cmdList []string, execOn uint8) {
		if execOn != execOnSuccess && execOn != execOnFailure {
			panic("Invalid option for exec-on")
		}
		for _, cmd := range cmdList {
			h, err := newExecPostHook(provider, execOn, cmd)
			if err != nil {
				logger.Errorf("Error initializing mirror %s: %s", cfg.Name, err.Error())
				panic(err)
			}
			provider.AddHook(h)
		}
	}

	// ExecOnSuccess hook
	addHookFromCmdList(cfg.ExecOnSuccess, execOnSuccess)

	// ExecOnFailure hook
	addHookFromCmdList(cfg.ExecOnFailure, execOnFailure)

	return provider
}
