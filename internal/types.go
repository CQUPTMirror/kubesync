package internal

import (
	"bytes"
	"encoding/json"

	"github.com/CQUPTMirror/kubesync/api/v1beta1"
)

type MirrorBase struct {
	ID        string `json:"id"`
	Namespace string `json:"namespace"`
}

// A MirrorStatus represents a msg when
// a worker has done syncing
type MirrorStatus struct {
	MirrorBase

	v1beta1.JobStatus
}

type MirrorConfig struct {
	MirrorBase

	v1beta1.JobSpec
}

type MirrorSchedule struct {
	MirrorBase

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
