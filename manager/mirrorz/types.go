package mirrorz

type MirrorZ struct {
	Version float64 `json:"version,omitempty"`
	Site    struct {
		Url          string `json:"url,omitempty"`
		Logo         string `json:"logo,omitempty"`
		LogoDarkmode string `json:"logo_darkmode,omitempty"`
		Abbr         string `json:"abbr,omitempty"`
		Name         string `json:"name,omitempty"`
		Homepage     string `json:"homepage,omitempty"`
		Issue        string `json:"issue,omitempty"`
		Request      string `json:"request,omitempty"`
		Email        string `json:"email,omitempty"`
		Group        string `json:"group,omitempty"`
		Disk         string `json:"disk,omitempty"`
		Note         string `json:"note,omitempty"`
		Big          string `json:"big,omitempty"`
		Disable      bool   `json:"disable,omitempty"`
	} `json:"site,omitempty"`
	Info      []Info   `json:"info,omitempty"`
	Mirrors   []Mirror `json:"mirrors,omitempty"`
	Extension string   `json:"extension,omitempty"`
	Endpoints []struct {
		Label   string   `json:"label,omitempty"`
		Public  bool     `json:"public,omitempty"`
		Resolve string   `json:"resolve,omitempty"`
		Filter  []string `json:"filter,omitempty"`
		Range   []string `json:"range,omitempty"`
	} `json:"endpoints,omitempty"`
}

type Info struct {
	Distro   string    `json:"distro,omitempty"`
	Category string    `json:"category,omitempty"`
	Urls     []InfoUrl `json:"urls,omitempty"`
}

type InfoUrl struct {
	Name string `json:"name,omitempty"`
	Url  string `json:"url,omitempty"`
}

type Mirror struct {
	Cname    string `json:"cname,omitempty"`
	Desc     string `json:"desc,omitempty"`
	Url      string `json:"url,omitempty"`
	Status   string `json:"status,omitempty"`
	Help     string `json:"help,omitempty"`
	Upstream string `json:"upstream,omitempty"`
	Size     string `json:"size,omitempty"`
	Disable  bool   `json:"disable,omitempty"`
}
