package mirrorz

type MirrorZ struct {
	Version float64 `json:"version"`
	Site    struct {
		Url          string `json:"url"`
		Logo         string `json:"logo"`
		LogoDarkmode string `json:"logo_darkmode,omitempty"`
		Abbr         string `json:"abbr,omitempty"`
		Name         string `json:"name"`
		Homepage     string `json:"homepage,omitempty"`
		Issue        string `json:"issue,omitempty"`
		Request      string `json:"request,omitempty"`
		Email        string `json:"email,omitempty"`
		Group        string `json:"group,omitempty"`
		Disk         string `json:"disk,omitempty"`
		Note         string `json:"note,omitempty"`
		Big          string `json:"big,omitempty"`
		Disable      bool   `json:"disable,omitempty"`
	} `json:"site"`
	Info      *[]Info   `json:"info"`
	Mirrors   *[]Mirror `json:"mirrors"`
	Extension string    `json:"extension,omitempty"`
	Endpoints []struct {
		Label   string   `json:"label"`
		Public  bool     `json:"public"`
		Resolve string   `json:"resolve"`
		Filter  []string `json:"filter,omitempty"`
		Range   []string `json:"range,omitempty"`
	} `json:"endpoints,omitempty"`
}

type Info struct {
	Distro   string    `json:"distro"`
	Category string    `json:"category"`
	Urls     []InfoUrl `json:"urls,omitempty"`
}

type InfoUrl struct {
	Name string `json:"name"`
	Url  string `json:"url"`
}

type Mirror struct {
	Cname    string `json:"cname"`
	Desc     string `json:"desc,omitempty"`
	Url      string `json:"url"`
	Status   string `json:"status"`
	Help     string `json:"help,omitempty"`
	Upstream string `json:"upstream,omitempty"`
	Size     string `json:"size,omitempty"`
	Disable  bool   `json:"disable,omitempty"`
}
