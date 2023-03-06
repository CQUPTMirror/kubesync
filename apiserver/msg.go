package apiserver

import (
	"bytes"
	"encoding/json"

	"github.com/ztelliot/kubesync/api/v1beta1"
)

// A MirrorStatus represents a msg when
// a worker has done syncing
type MirrorStatus struct {
	ID string `json:"id"`

	v1beta1.JobStatus
}

type MirrorConfig struct {
	ID string `json:"id"`

	v1beta1.JobSpec
}

type MirrorSchedules struct {
	Schedules []MirrorSchedule `json:"schedules"`
}

type MirrorSchedule struct {
	MirrorID     string `json:"id"`
	NextSchedule int64  `json:"next_schedule"`
}

// A CmdVerb is an action to a job or worker
type CmdVerb uint8

const (
	// CmdStart start a job
	CmdStart CmdVerb = iota
	// CmdStop stop syncing, but keep the job
	CmdStop
	// CmdDisable disable the job (stops goroutine)
	CmdDisable
	// CmdRestart restart a syncing job
	CmdRestart
	// CmdPing ensures the goroutine is alive
	CmdPing

	// CmdReload tells a worker to reload mirror config
	CmdReload
)

func (c CmdVerb) String() string {
	mapping := map[CmdVerb]string{
		CmdStart:   "start",
		CmdStop:    "stop",
		CmdDisable: "disable",
		CmdRestart: "restart",
		CmdPing:    "ping",
		CmdReload:  "reload",
	}
	return mapping[c]
}

func NewCmdVerbFromString(s string) CmdVerb {
	mapping := map[string]CmdVerb{
		"start":   CmdStart,
		"stop":    CmdStop,
		"disable": CmdDisable,
		"restart": CmdRestart,
		"ping":    CmdPing,
		"reload":  CmdReload,
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
	Cmd     CmdVerb         `json:"cmd"`
	Args    []string        `json:"args"`
	Options map[string]bool `json:"options"`
}
