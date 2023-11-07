package external

import (
	"encoding/json"
	"errors"
	"github.com/CQUPTMirror/kubesync/api/v1beta1"
	"github.com/CQUPTMirror/kubesync/internal"
	"github.com/xhit/go-str2duration/v2"
	"io"
	"net/http"
	"net/url"
	"time"
)

type giteaRepo struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	FullName    string `json:"full_name"`
	Desc        string `json:"description,omitempty"`
	Empty       bool   `json:"empty,omitempty"`
	Size        uint64 `json:"size,omitempty"`
	CloneUrl    string `json:"clone_url,omitempty"`
	OriginalUrl string `json:"original_url,omitempty"`
	Interval    string `json:"mirror_interval,omitempty"`
	Updated     string `json:"mirror_updated,omitempty"`
}

func (r *giteaRepo) getStatus() v1beta1.SyncStatus {
	if r.Empty {
		return v1beta1.Syncing
	} else {
		return v1beta1.Success
	}
}

func (r *giteaRepo) getTime() *time.Time {
	t, err := time.Parse(time.RFC3339, r.Updated)
	if err != nil {
		return nil
	}
	return &t
}

type giteaMsg struct {
	OK   bool        `json:"ok"`
	Data []giteaRepo `json:"data"`
}

type giteaProvider struct {
	url string
	hc  *http.Client
}

func (p *giteaProvider) List() ([]internal.MirrorStatus, error) {
	u, err := url.Parse("/api/v1/repos/search")
	if err != nil {
		return nil, err
	}
	u.RawQuery = "mode=mirror"
	base, err := url.Parse(p.url)
	if err != nil {
		return nil, err
	}
	resp, err := p.hc.Get(base.ResolveReference(u).String())
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("HTTP status code is not 200")
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	info := new(giteaMsg)
	err = json.Unmarshal(body, info)
	if err != nil || !info.OK {
		return nil, err
	}

	var ws []internal.MirrorStatus
	for _, v := range info.Data {
		t := v.getTime()
		i, _ := str2duration.ParseDuration(v.Interval)
		ws = append(ws, internal.MirrorStatus{
			ID:      v.Name,
			Desc:    v.Desc,
			Url:     v.CloneUrl,
			Type:    "git",
			SizeStr: internal.ParseSize(v.Size * internal.K),
			JobStatus: v1beta1.JobStatus{
				Status:     v.getStatus(),
				LastUpdate: t.Unix(),
				Scheduled:  t.Add(i).Unix(),
				Upstream:   v.OriginalUrl,
				Size:       v.Size * internal.K,
			},
		})
	}

	return ws, nil
}
