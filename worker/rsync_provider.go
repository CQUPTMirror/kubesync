package worker

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

type rsyncConfig struct {
	name                        string
	rsyncCmd                    string
	upstreamURL, excludeFile    string
	extraOptions                []string
	overriddenOptions           []string
	rsyncNeverTimeout           bool
	rsyncTimeoutValue           int
	workingDir, logDir, logFile string
	useIPv6, useIPv4            bool
	interval                    time.Duration
	retry                       int
	timeout                     time.Duration
}

// An RsyncProvider provides the implementation to rsync-based syncing jobs
type rsyncProvider struct {
	baseProvider
	rsyncConfig
	options  []string
	dataSize uint64
}

func newRsyncProvider(c rsyncConfig) (*rsyncProvider, error) {
	// TODO: check config options
	if !strings.HasSuffix(c.upstreamURL, "/") {
		return nil, errors.New("rsync upstream URL should ends with /")
	}
	if c.retry == 0 {
		c.retry = defaultMaxRetry
	}
	provider := &rsyncProvider{
		baseProvider: baseProvider{
			name:     c.name,
			ctx:      NewContext(),
			interval: c.interval,
			retry:    c.retry,
			timeout:  c.timeout,
		},
		rsyncConfig: c,
	}

	if c.rsyncCmd == "" {
		provider.rsyncCmd = "rsync"
	}

	options := []string{
		"-aHvh", "--no-o", "--no-g", "--stats",
		"--filter", "risk .~tmp~/", "--exclude", ".~tmp~/",
		"--delete", "--delete-after", "--delay-updates",
		"--safe-links",
	}
	if c.overriddenOptions != nil {
		options = c.overriddenOptions
	}

	if !c.rsyncNeverTimeout {
		timeo := 120
		if c.rsyncTimeoutValue > 0 {
			timeo = c.rsyncTimeoutValue
		}
		options = append(options, fmt.Sprintf("--timeout=%d", timeo))
	}

	if c.useIPv6 {
		options = append(options, "-6")
	} else if c.useIPv4 {
		options = append(options, "-4")
	}

	if c.excludeFile != "" {
		options = append(options, "--exclude-from", c.excludeFile)
	}
	if c.extraOptions != nil {
		options = append(options, c.extraOptions...)
	}
	provider.options = options

	provider.ctx.Set(_WorkingDirKey, c.workingDir)
	provider.ctx.Set(_LogDirKey, c.logDir)
	provider.ctx.Set(_LogFileKey, c.logFile)

	return provider, nil
}

func (p *rsyncProvider) Upstream() string {
	return p.upstreamURL
}

func (p *rsyncProvider) DataSize() uint64 {
	return p.dataSize
}

func (p *rsyncProvider) Run(started chan empty) error {
	p.dataSize = 0
	defer p.closeLogFile()
	if err := p.Start(); err != nil {
		return err
	}
	started <- empty{}
	if err := p.Wait(); err != nil {
		code, msg := TranslateRsyncErrorCode(err)
		if code != 0 {
			logger.Debug("Rsync exitcode %d (%s)", code, msg)
			if p.logFileFd != nil {
				p.logFileFd.WriteString(msg + "\n")
			}
		}
		return err
	}
	p.dataSize = ExtractSizeFromRsyncLog(p.LogFile())
	return nil
}

func (p *rsyncProvider) Start() error {
	p.Lock()
	defer p.Unlock()

	if p.IsRunning() {
		return errors.New("provider is currently running")
	}

	command := []string{p.rsyncCmd}
	command = append(command, p.options...)
	command = append(command, p.upstreamURL, p.WorkingDir())

	p.cmd = newCmdJob(p, command, p.WorkingDir(), nil)
	if err := p.prepareLogFile(false); err != nil {
		return err
	}

	if err := p.cmd.Start(); err != nil {
		return err
	}
	p.isRunning.Store(true)
	logger.Debugf("set isRunning to true: %s", p.Name())
	return nil
}
