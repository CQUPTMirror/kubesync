package external

import (
	"github.com/CQUPTMirror/kubesync/api/v1beta1"
	"github.com/CQUPTMirror/kubesync/internal"
	"net/http"
)

type External interface {
	List() ([]internal.MirrorStatus, error)
	// TODO: Add API support to manage external jobs
	//Get(id string) (v1beta1.JobStatus, error)
	//Delete(id string) error
	//Create(id string) error
}

func Provider(cfg *v1beta1.JobConfig, hc *http.Client) External {
	var provider External

	switch cfg.Provider {
	case "gitea":
		provider = &giteaProvider{url: cfg.Upstream, hc: hc}
	default:
		return nil
	}

	return provider
}
