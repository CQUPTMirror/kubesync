package worker

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"regexp"
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

// PostJSON posts json object to url
func PostJSON(url string, obj interface{}, client *http.Client) (*http.Response, error) {
	if client == nil {
		client, _ = CreateHTTPClient()
	}
	b := new(bytes.Buffer)
	if err := json.NewEncoder(b).Encode(obj); err != nil {
		return nil, err
	}
	return client.Post(url, "application/json; charset=utf-8", b)
}

// GetJSON gets a json response from url
func GetJSON(url string, obj interface{}, client *http.Client) (*http.Response, error) {
	if client == nil {
		client, _ = CreateHTTPClient()
	}

	resp, err := client.Get(url)
	if err != nil {
		return resp, err
	}
	if resp.StatusCode != http.StatusOK {
		return resp, errors.New("HTTP status code is not 200")
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
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
	if content, err := ioutil.ReadFile(fileName); err == nil {
		matches = re.FindAllSubmatch(content, -1)
		// fmt.Printf("FindAllSubmatchInFile: %q\n", matches)
	}
	return
}

// ExtractSizeFromLog uses a regexp to extract the size from log files
func ExtractSizeFromLog(logFile string, re *regexp.Regexp) string {
	matches, _ := FindAllSubmatchInFile(logFile, re)
	if matches == nil || len(matches) == 0 {
		return ""
	}
	// return the first capture group of the last occurrence
	return string(matches[len(matches)-1][1])
}

// ExtractSizeFromRsyncLog extracts the size from rsync logs
func ExtractSizeFromRsyncLog(logFile string) string {
	// (?m) flag enables multi-line mode
	re := regexp.MustCompile(`(?m)^Total file size: ([0-9\.]+[KMGTP]?) bytes`)
	return ExtractSizeFromLog(logFile, re)
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

func GetEnv(key, def string) string {
	val, ex := os.LookupEnv(key)
	if !ex {
		return def
	}
	return val
}
