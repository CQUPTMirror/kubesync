package worker

import (
	"errors"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

// runner is to run os commands giving command line, env and log file
// it's an alternative to python-sh or go-sh

var errProcessNotStarted = errors.New("Process Not Started")

type cmdJob struct {
	sync.Mutex
	cmd        *exec.Cmd
	workingDir string
	env        map[string]string
	logFile    *os.File
	finished   chan empty
	provider   mirrorProvider
	retErr     error
}

func newCmdJob(provider mirrorProvider, cmdAndArgs []string, workingDir string, env map[string]string) *cmdJob {
	var cmd *exec.Cmd

	if len(cmdAndArgs) == 1 {
		cmd = exec.Command(cmdAndArgs[0])
	} else if len(cmdAndArgs) > 1 {
		c := cmdAndArgs[0]
		args := cmdAndArgs[1:]
		cmd = exec.Command(c, args...)
	} else if len(cmdAndArgs) == 0 {
		panic("Command length should be at least 1!")
	}

	logger.Debugf("Executing command %s at %s", cmdAndArgs[0], workingDir)
	if _, err := os.Stat(workingDir); os.IsNotExist(err) {
		logger.Debugf("Making dir %s", workingDir)
		if err = os.MkdirAll(workingDir, 0755); err != nil {
			logger.Errorf("Error making dir %s: %s", workingDir, err.Error())
		}
	}
	cmd.Dir = workingDir
	cmd.Env = newEnviron(env, true)

	return &cmdJob{
		cmd:        cmd,
		workingDir: workingDir,
		env:        env,
		provider:   provider,
	}
}

func (c *cmdJob) Start() error {
	logger.Debugf("Command start: %v", c.cmd.Args)
	c.finished = make(chan empty, 1)

	if err := c.cmd.Start(); err != nil {
		return err
	}

	return nil
}

func (c *cmdJob) Wait() error {
	c.Lock()
	defer c.Unlock()

	select {
	case <-c.finished:
		return c.retErr
	default:
		err := c.cmd.Wait()
		c.retErr = err
		close(c.finished)
		return err
	}
}

func (c *cmdJob) SetLogFile(logFile *os.File) {
	c.cmd.Stdout = logFile
	c.cmd.Stderr = logFile
}

func (c *cmdJob) Terminate() error {
	if c.cmd == nil || c.cmd.Process == nil {
		return errProcessNotStarted
	}

	err := unix.Kill(c.cmd.Process.Pid, syscall.SIGTERM)
	if err != nil {
		return err
	}

	select {
	case <-time.After(2 * time.Second):
		unix.Kill(c.cmd.Process.Pid, syscall.SIGKILL)
		logger.Warningf("SIGTERM failed to kill the job in 2s. SIGKILL sent")
	case <-c.finished:
	}
	return nil
}

// Copied from go-sh
func newEnviron(env map[string]string, inherit bool) []string { //map[string]string {
	environ := make([]string, 0, len(env))
	if inherit {
		for _, line := range os.Environ() {
			// if os environment and env collapses,
			// omit the os one
			k := strings.Split(line, "=")[0]
			if _, ok := env[k]; ok {
				continue
			}
			environ = append(environ, line)
		}
	}
	for k, v := range env {
		environ = append(environ, k+"="+v)
	}
	return environ
}
