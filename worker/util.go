package worker

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/CQUPTMirror/kubesync/internal"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
)

var rsyncExitValues = map[int]string{
	0:  "Success",
	1:  "Syntax or usage error",
	2:  "Protocol incompatibility",
	3:  "Errors selecting input/output files, dirs",
	4:  "Requested action not supported: an attempt was made to manipulate 64-bit files on a platform that cannot support them; or an option was specified that is supported by the client and not by the server.",
	5:  "Error starting client-server protocol",
	6:  "Daemon unable to append to log-file",
	10: "Error in socket I/O",
	11: "Error in file I/O",
	12: "Error in rsync protocol data stream",
	13: "Errors with program diagnostics",
	14: "Error in IPC code",
	20: "Received SIGUSR1 or SIGINT",
	21: "Some error returned by waitpid()",
	22: "Error allocating core memory buffers",
	23: "Partial transfer due to error",
	24: "Partial transfer due to vanished source files",
	25: "The --max-delete limit stopped deletions",
	30: "Timeout in data send/receive",
	35: "Timeout waiting for daemon connection",
}

// CreateHTTPClient returns a http.Client
func CreateHTTPClient() (*http.Client, error) {
	var tlsConfig *tls.Config

	tr := &http.Transport{
		MaxIdleConnsPerHost: 20,
		TLSClientConfig:     tlsConfig,
	}

	return &http.Client{
		Transport: tr,
		Timeout:   5 * time.Second,
	}, nil
}

// HandleRequest post/head url
func (w *Worker) HandleRequest(method, url string, obj interface{}) (*http.Response, error) {
	var req *http.Request
	var err error

	if obj != nil {
		var b *bytes.Buffer
		b = new(bytes.Buffer)
		if err := json.NewEncoder(b).Encode(obj); err != nil {
			return nil, err
		}
		req, err = http.NewRequest(method, url, b)
	} else {
		req, err = http.NewRequest(method, url, nil)
	}

	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	return w.httpClient.Do(req)
}

// GetJSON gets a json response from url
func (w *Worker) GetJSON(url string, obj interface{}) (*http.Response, error) {
	resp, err := w.httpClient.Get(url)
	if err != nil {
		return resp, err
	}
	if resp.StatusCode != http.StatusOK {
		return resp, errors.New("HTTP status code is not 200")
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp, err
	}
	return resp, json.Unmarshal(body, obj)
}

// FindAllSubmatchInFile calls re.FindAllSubmatch to find matches in given file
func FindAllSubmatchInFile(fileName string, re *regexp.Regexp) (matches [][][]byte, err error) {
	if fileName == "/dev/null" {
		err = errors.New("Invalid log file")
		return
	}
	if content, err := os.ReadFile(fileName); err == nil {
		matches = re.FindAllSubmatch(content, -1)
		// fmt.Printf("FindAllSubmatchInFile: %q\n", matches)
	}
	return
}

// ExtractSizeFromLog uses a regexp to extract the size from log files
func ExtractSizeFromLog(logFile string, re *regexp.Regexp) uint64 {
	matches, _ := FindAllSubmatchInFile(logFile, re)
	if matches == nil || len(matches) == 0 {
		return 0
	}
	// return the first capture group of the last occurrence
	return internal.ParseSizeStr(string(matches[len(matches)-1][1]))
}

// ExtractSizeFromRsyncLog extracts the size from rsync logs
func ExtractSizeFromRsyncLog(logFile string) uint64 {
	// (?m) flag enables multi-line mode
	re := regexp.MustCompile(`(?m)^Total file size: ([0-9\.]+[KMGTP]?) bytes`)
	return ExtractSizeFromLog(logFile, re)
}

// ExtractSizeFromWalk extracts the size from path
func ExtractSizeFromWalk(path string) uint64 {
	sizes := make(chan int64)
	readSize := func(path string, file os.FileInfo, err error) error {
		if err != nil || file == nil {
			return nil
		}
		if !file.IsDir() {
			sizes <- file.Size()
		}
		return nil
	}

	go func() {
		filepath.Walk(path, readSize)
		close(sizes)
	}()

	used := int64(0)
	for s := range sizes {
		used += s
	}

	return uint64(used)
}

// ExtractSizeFromFileSystem extracts the size from filesystem
func ExtractSizeFromFileSystem(path string) uint64 {
	fs := syscall.Statfs_t{}
	err := syscall.Statfs(path, &fs)
	if err != nil {
		return 0
	}

	return (fs.Blocks - fs.Bfree) * uint64(fs.Bsize)
}

// TranslateRsyncErrorCode translates the exit code of rsync to a message
func TranslateRsyncErrorCode(cmdErr error) (exitCode int, msg string) {

	if exiterr, ok := cmdErr.(*exec.ExitError); ok {
		exitCode = exiterr.ExitCode()
		strerr, valid := rsyncExitValues[exitCode]
		if valid {
			msg = fmt.Sprintf("rsync error: %s", strerr)
		}
	}
	return
}

func GetStringEnv(key, def string) string {
	val, ex := os.LookupEnv(key)
	if !ex || val == "" {
		return def
	}
	return val
}

func GetIntEnv(key string, def int) int {
	val, ex := os.LookupEnv(key)
	if !ex {
		return def
	}
	if i, err := strconv.Atoi(val); err != nil || i == 0 {
		return def
	} else {
		return i
	}
}

func GetBoolEnv(key string) bool {
	val, ex := os.LookupEnv(key)
	if !ex {
		return false
	}
	if s, err := strconv.ParseBool(val); err != nil {
		return false
	} else {
		return s
	}
}

func GetListEnv(key string) []string {
	val, ex := os.LookupEnv(key)
	if !ex || val == "" {
		return nil
	}
	return strings.Split(val, ";")
}
