package internal

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/CQUPTMirror/kubesync/api/v1beta1"
)

const (
	B = 1
	K = 1024 * B
	M = 1024 * K
	G = 1024 * M
	T = 1024 * G
)

type AnnouncementInfo struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Author  string `json:"author"`
	Content string `json:"content"`

	v1beta1.AnnouncementStatus
}

type FileBase struct {
	Type  v1beta1.FileType `json:"type"`
	Alias string           `json:"alias"`
	Files []string         `json:"files"`
}

type FileInfo struct {
	ID    string           `json:"id"`
	Type  v1beta1.FileType `json:"type"`
	Alias string           `json:"alias"`

	v1beta1.FileStatus
}

type MirrorStatus struct {
	ID      string             `json:"id"`
	Alias   string             `json:"alias"`
	Desc    string             `json:"desc"`
	Url     string             `json:"url"`
	HelpUrl string             `json:"helpUrl"`
	Type    v1beta1.MirrorType `json:"type"`
	SizeStr string             `json:"sizeStr"`

	v1beta1.JobStatus
}

type MirrorConfig struct {
	ID string `json:"id"`

	v1beta1.JobSpec
}

type MirrorSchedule struct {
	NextSchedule int64 `json:"next_schedule"`
}

// A CmdVerb is an action to a job or worker
type CmdVerb uint8

const (
	// CmdStart start a job
	CmdStart CmdVerb = iota
	// CmdStop stop syncing, but keep the job
	CmdStop
	// CmdRestart restart a syncing job
	CmdRestart
	// CmdPing ensures the goroutine is alive
	CmdPing
	// CmdUpdate update size
	CmdUpdate
)

func (c CmdVerb) String() string {
	mapping := map[CmdVerb]string{
		CmdStart:   "start",
		CmdStop:    "stop",
		CmdRestart: "restart",
		CmdPing:    "ping",
	}
	return mapping[c]
}

func NewCmdVerbFromString(s string) CmdVerb {
	mapping := map[string]CmdVerb{
		"start":   CmdStart,
		"stop":    CmdStop,
		"restart": CmdRestart,
		"ping":    CmdPing,
	}
	return mapping[s]
}

// Marshal and Unmarshal for CmdVerb
func (s CmdVerb) MarshalJSON() ([]byte, error) {
	buffer := bytes.NewBufferString(`"`)
	buffer.WriteString(s.String())
	buffer.WriteString(`"`)
	return buffer.Bytes(), nil
}

func (s *CmdVerb) UnmarshalJSON(b []byte) error {
	var j string
	err := json.Unmarshal(b, &j)
	if err != nil {
		return err
	}
	*s = NewCmdVerbFromString(j)
	return nil
}

// A ClientCmd is the command message send from client
// to the manager
type ClientCmd struct {
	Cmd   CmdVerb `json:"cmd"`
	Force bool    `json:"force"`
}

func ParseSize(size uint64) (sizeStr string) {
	switch {
	case size > T:
		sizeStr = fmt.Sprintf("%.2fT", float64(size)/float64(T))
	case size > G:
		sizeStr = fmt.Sprintf("%.2fG", float64(size)/float64(G))
	case size > M:
		sizeStr = fmt.Sprintf("%.2fM", float64(size)/float64(M))
	case size > K:
		sizeStr = fmt.Sprintf("%.2fK", float64(size)/float64(K))
	default:
		sizeStr = fmt.Sprintf("%dB", size)
	}
	return
}

func ParseSizeStr(sizeStr string) (size uint64) {
	if len(sizeStr) > 0 && sizeStr != "unknown" {
		isBit := false
		sizeStr = strings.ReplaceAll(sizeStr, " ", "")
		sizeStr = strings.ReplaceAll(sizeStr, "\n", "")
		if strings.HasSuffix(sizeStr, "b") {
			isBit = true
			sizeStr = strings.TrimSuffix(sizeStr, "b")
		} else {
			sizeStr = strings.TrimSuffix(sizeStr, "B")
		}
		sizeStr = strings.TrimSuffix(sizeStr, "i")
		sizeStr = strings.ToUpper(sizeStr)
		if len(sizeStr) > 0 {
			sizeRaw, err := strconv.ParseFloat(sizeStr[:len(sizeStr)-1], 64)
			if err != nil {
				return
			}
			switch sizeStr[len(sizeStr)-1] {
			case 'T':
				sizeRaw *= T
			case 'G':
				sizeRaw *= G
			case 'M':
				sizeRaw *= M
			case 'K':
				sizeRaw *= K
			default:
				if sizeRaw, err = strconv.ParseFloat(sizeStr, 64); err != nil {
					return
				}
			}
			if isBit {
				size = uint64(sizeRaw / 8)
			} else {
				size = uint64(sizeRaw)
			}
		}
	}
	return
}
