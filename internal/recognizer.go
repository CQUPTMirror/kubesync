package internal

import (
	"fmt"
	"github.com/CQUPTMirror/kubesync/api/v1beta1"
	"path"
	"strconv"
	"strings"
)

func combine(s []string, e string) string {
	r := ""
	for _, v := range s {
		if v != "" {
			r += v + e
		}
	}
	return strings.TrimSuffix(r, e)
}

func Recognizer(filepath string) (f v1beta1.FileInfo) {
	f.Path = filepath
	name := path.Base(filepath)
	f.Ext = path.Ext(filepath)
	name = strings.TrimSuffix(name, f.Ext)
	switch {
	case strings.HasPrefix(name, "CentOS-"):
		name = strings.TrimPrefix(name, "CentOS-")
		stream := false
		if strings.HasPrefix(name, "Stream-") {
			stream = true
			name = strings.TrimPrefix(name, "Stream-")
		}
		nameSp := strings.Split(name, "-")
		f.MajorVersion = nameSp[0]
		if len(nameSp) > 3 {
			switch f.MajorVersion {
			case "6":
				fallthrough
			case "7":
				f.Arch = nameSp[1]
				f.Edition = nameSp[2]
				f.Version = strings.Join(nameSp[3:], "-")
			default:
				f.Version = nameSp[1]
				if stream {
					f.Version = fmt.Sprintf("Stream-%s", nameSp[1])
				}
				f.Arch = nameSp[2]
				f.Edition = nameSp[3]
			}
		}
	case strings.HasPrefix(name, "debian-"):
		name = strings.TrimPrefix(name, "debian-")
		nameSp := strings.Split(name, "-")
		if len(nameSp) >= 3 {
			start := 0
			if '0' > nameSp[start][0] || nameSp[start][0] > '9' {
				f.Edition = nameSp[start]
				start += 1
			}
			f.Version = nameSp[start]
			f.Arch = nameSp[start+1]
			if len(nameSp) >= start+3 {
				f.EditionType = nameSp[start+2]
				if f.Edition == "live" {
					f.Edition, f.EditionType = f.EditionType, f.Edition
				}
				if f.Arch == "source" {
					f.Edition, f.Arch = f.Arch, ""
				}
				if len(nameSp) == start+4 {
					f.Part, _ = strconv.Atoi(nameSp[start+3])
				}
			}
		}
	case strings.HasPrefix(name, "edubuntu-"):
		fallthrough
	case strings.HasPrefix(name, "kubuntu-"):
		fallthrough
	case strings.HasPrefix(name, "lubuntu-"):
		fallthrough
	case strings.HasPrefix(name, "mythbuntu-"):
		fallthrough
	case strings.HasPrefix(name, "ubuntucinnamon-"):
		fallthrough
	case strings.HasPrefix(name, "ubuntukylin-"):
		fallthrough
	case strings.HasPrefix(name, "ubuntustudio-"):
		fallthrough
	case strings.HasPrefix(name, "xubuntu-"):
		nameSp := strings.Split(name, "-")
		if len(nameSp) >= 4 {
			f.MajorVersion = nameSp[0]
			f.Version = nameSp[1]
			f.Edition = nameSp[2]
			start := 3
			if f.Edition == "beta" {
				f.Version = fmt.Sprintf("%s-%s", nameSp[1], nameSp[2])
				f.Edition = nameSp[start]
				start += 1
			}
			if len(nameSp) >= start+1 {
				f.Arch = nameSp[start]
			}
		}
	case strings.HasPrefix(name, "ubuntu-budgie-"):
		fallthrough
	case strings.HasPrefix(name, "ubuntu-gnome-"):
		fallthrough
	case strings.HasPrefix(name, "ubuntu-mate-"):
		fallthrough
	case strings.HasPrefix(name, "ubuntu-unity-"):
		nameSp := strings.Split(name, "-")
		if len(nameSp) >= 5 {
			f.MajorVersion = fmt.Sprintf("%s-%s", nameSp[0], nameSp[1])
			f.Version = nameSp[2]
			f.Edition = nameSp[3]
			start := 4
			if f.Edition == "beta" {
				f.Version = fmt.Sprintf("%s-%s", nameSp[2], nameSp[3])
				f.Edition = nameSp[start]
				start += 1
			}
			if len(nameSp) >= start+1 {
				f.Arch = nameSp[start]
			}
		}
	case strings.HasPrefix(name, "ubuntu-"):
		name = strings.TrimPrefix(name, "ubuntu-")
		nameSp := strings.Split(name, "-")
		if len(nameSp) >= 3 {
			f.Version = nameSp[0]
			start := 1
			if nameSp[start] == "beta" {
				f.Version = fmt.Sprintf("%s-beta", nameSp[0])
				start += 1
			}
			if nameSp[start] == "live" {
				start += 1
			}
			if nameSp[start] == "src" {
				f.Edition = "src"
				f.Part, _ = strconv.Atoi(nameSp[start+1])
			} else {
				if len(nameSp) >= start+2 {
					f.Edition = nameSp[start]
					if f.Edition == "legacy" {
						f.Edition = fmt.Sprintf("%s-%s", f.Edition, nameSp[start+1])
						start += 1
					}
					f.Arch = nameSp[start+1]
					if f.Arch == "legacy" {
						f.Edition = fmt.Sprintf("%s-%s", f.Edition, "legacy")
						f.Arch = nameSp[start+2]
					}
					if strings.HasSuffix(f.Arch, "+intel") {
						f.Arch = fmt.Sprintf("%s-iot", f.Arch)
					}
				}
			}
		}
	case strings.HasPrefix(name, "Fedora-"):
		name = strings.TrimPrefix(name, "Fedora-")
		nameSp := strings.Split(name, "-")
		if len(nameSp) >= 5 {
			f.Edition = nameSp[0]
			f.EditionType = nameSp[1]
			f.Arch = nameSp[2]
			f.MajorVersion = nameSp[3]
			f.Version = nameSp[4]
		}
	case strings.HasPrefix(name, "deepin-"):
		name = strings.TrimPrefix(name, "deepin-")
		nameSp := strings.Split(name, "-")
		if len(nameSp) >= 4 {
			f.Edition = nameSp[0]
			f.EditionType = nameSp[1]
			f.Version = strings.Join(nameSp[2:len(nameSp)-1], "-")
			f.Arch = nameSp[len(nameSp)-1]
		}
	case strings.HasPrefix(name, "kali-linux-"):
		name = strings.TrimPrefix(name, "kali-linux-")
		nameSp := strings.Split(name, "-")
		f.Version = nameSp[0]
		if len(nameSp) >= 3 {
			start := 1
			if nameSp[1][0] == 'W' {
				f.Version = fmt.Sprintf("%s-%s", nameSp[0], nameSp[1])
				start += 1
			}
			switch len(nameSp) {
			case start + 2:
				f.Edition = nameSp[start]
				if f.Edition == "installer" {
					f.Edition = ""
				}
				f.Arch = nameSp[start+1]
			case start + 3:
				f.Edition = nameSp[start+1]
				f.Arch = nameSp[start+2]
			}
		}
	case strings.HasPrefix(name, "openSUSE-"):
		if !strings.Contains(name, "Micro") && strings.HasSuffix(name, "-Current") {
			name = strings.TrimPrefix(name, "openSUSE-")
			nameSp := strings.Split(name, "-")
			f.MajorVersion = nameSp[0]
			switch f.MajorVersion {
			case "Leap":
				if len(nameSp) >= 3 {
					f.Version = nameSp[1]
					f.EditionType = nameSp[2]
					start := 3
					switch f.EditionType {
					case "Rescue":
						start += 1
					case "CR":
						f.EditionType = nameSp[start]
						start += 1
					default:
						if _, err := strconv.Atoi(f.EditionType); err == nil {
							f.Version = fmt.Sprintf("%s-%s", f.Version, f.EditionType)
							f.EditionType = nameSp[start]
							start += 1
						}
					}
					if len(nameSp) >= start+1 {
						if nameSp[start] == "Live" {
							start += 1
						}
						f.Arch = nameSp[start]
					}
				}
			case "Tumbleweed":
				if len(nameSp) >= 2 {
					f.EditionType = nameSp[1]
					start := 2
					if f.EditionType == "Rescue" {
						f.EditionType = fmt.Sprintf("%s-%s", nameSp[1], nameSp[2])
						start += 1
					}
					if len(nameSp) > start && nameSp[start] == "Live" {
						start += 1
					}
					if strings.HasPrefix(f.EditionType, "Yomi") {
						etSp := strings.Split(f.EditionType, ".")
						if len(etSp) == 2 {
							f.Arch = etSp[1]
							f.EditionType = etSp[0]
						}
					} else {
						f.Arch = nameSp[start]
					}
				}
			case "Kubic":
				if len(nameSp) >= 3 {
					f.EditionType = nameSp[1]
					f.Arch = nameSp[2]
				}
			}
		}
	case strings.HasPrefix(name, "archlinux-"):
		name = strings.TrimPrefix(name, "archlinux-")
		nameSp := strings.Split(name, "-")
		if len(nameSp) > 1 {
			f.Version = nameSp[0]
			f.Arch = nameSp[1]
		}
	case strings.HasPrefix(name, "alpine-"):
		name = strings.TrimPrefix(name, "alpine-")
		nameSp := strings.Split(name, "-")
		switch len(nameSp) {
		case 2:
			f.Version = nameSp[0]
			f.Arch = nameSp[1]
		case 3:
			f.Edition = nameSp[0]
			f.Version = nameSp[1]
			f.Arch = nameSp[2]
		}
	case strings.HasPrefix(name, "proxmox-"):
		nameSp := strings.Split(name, "_")
		if len(nameSp) == 2 {
			f.MajorVersion = nameSp[0]
			f.Version = nameSp[1]
		}
	case strings.HasPrefix(name, "AlmaLinux-"):
		name = strings.TrimPrefix(name, "AlmaLinux-")
		nameSp := strings.Split(name, "-")
		f.Version = nameSp[0]
		if len(nameSp) >= 3 {
			start := 1
			if nameSp[start] == "latest" {
				start += 1
			}
			f.Arch = nameSp[start+1]
			f.EditionType = nameSp[start+2]
			if f.EditionType == "Live" {
				f.Edition = strings.Join(nameSp[start+3:], "-")
			}
		}
	case strings.HasPrefix(name, "texlive"):
		name = strings.TrimPrefix(name, "texlive")
		nameSp := strings.Split(name, "-")
		if len(nameSp) > 1 {
			f.Version = nameSp[1]
		}
	}
	part := ""
	if f.Part > 0 {
		part = fmt.Sprintf("Part %d", f.Part)
	}
	if f.MajorVersion != "" || f.Version != "" {
		f.Name = fmt.Sprintf("%s (%s)", combine([]string{f.MajorVersion, f.Version}, " "), combine([]string{f.Arch, f.Edition, f.EditionType, part}, ", "))
		if strings.HasSuffix(f.Name, "()") {
			f.Name = strings.TrimSuffix(f.Name, " ()")
		}
	}
	return f
}
