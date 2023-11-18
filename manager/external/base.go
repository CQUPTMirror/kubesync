package external

import (
	"github.com/CQUPTMirror/kubesync/api/v1beta1"
	"github.com/CQUPTMirror/kubesync/internal"
	"github.com/CQUPTMirror/kubesync/manager/mirrorz"
	"net/http"
)

type External interface {
	List() ([]internal.MirrorStatus, error)
	ListZ() ([]mirrorz.Mirror, error)
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
