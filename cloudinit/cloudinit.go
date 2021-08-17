package cloudinit

import (
	"fmt"

	"github.com/michaelhenkel/cn2kubevirt/roles"
	"gopkg.in/yaml.v3"
)

type cloudInit struct {
	Hostname       string            `yaml:"hostname"`
	ManageEtcHosts bool              `yaml:"manage_etc_hosts"`
	Users          []instanceUser    `yaml:"users"`
	SSHPwauth      bool              `yaml:"ssh_pwauth"`
	DisableRoot    bool              `yaml:"disable_root"`
	Chpasswd       chpasswd          `yaml:"chpasswd"`
	WriteFiles     []writeFiles      `yaml:"write_files"`
	RunCMD         []string          `yaml:"runcmd"`
	Snap           map[string]string `yaml:"snap"`
	Network        netw              `yaml:"network"`
	Apt            apt               `yaml:"apt"`
}

type netw struct {
	Version   string              `yaml:"version"`
	Ethernets map[string]ethernet `yaml:"ethernets"`
}

type ethernet struct {
	Match map[string]string `yaml:"match"`
	Dhcp4 bool              `yaml:"dhcp4"`
}

type chpasswd struct {
	List   string `yaml:"list"`
	Expire bool   `yaml:"expire"`
}

type writeFiles struct {
	Content string `yaml:"content"`
	Path    string `yaml:"path"`
}

type apt struct {
	Primary []primary `yaml:"primary"`
}

type primary struct {
	Arches []string `yaml:"arches"`
	Uri    string   `yaml:"uri"`
}

type instanceUser struct {
	Name              string   `yaml:"name"`
	Sudo              string   `yaml:"sudo"`
	Groups            string   `yaml:"groups"`
	Home              string   `yaml:"home"`
	Shell             string   `yaml:"shell"`
	LockPasswd        bool     `yaml:"lock_passwd"`
	SSHAuthorizedKeys []string `yaml:"ssh-authorized-keys"`
}

func CreateCloudInit(hostname, key, gateway, criomirror, dns, registry string, routes []string, distro roles.Distro) (string, error) {
	ci := cloudInit{
		Hostname:       hostname,
		ManageEtcHosts: true,
		Users: []instanceUser{{
			Name:              "contrail",
			Sudo:              "ALL=(ALL) NOPASSWD:ALL",
			Home:              "/home/contrail",
			Shell:             "/bin/bash",
			LockPasswd:        false,
			SSHAuthorizedKeys: []string{key},
		}, {
			Name:              "root",
			Sudo:              "ALL=(ALL) NOPASSWD:ALL",
			SSHAuthorizedKeys: []string{key},
		}},
		SSHPwauth:   true,
		DisableRoot: false,
		Chpasswd: chpasswd{
			List: `contrail:contrail
root:contrail`,
			Expire: false,
		},
		WriteFiles: []writeFiles{{
			Content: `[Resolve]
DNS=` + dns,
			Path: "/etc/systemd/resolved.conf",
		}},
		RunCMD: []string{
			"systemctl restart systemd-resolved.service",
			`echo "10.160.12.173 svl-artifactory.juniper.net" >> /etc/hosts`,
		},
	}

	switch distro {
	case roles.Ubuntu:
		content := `network:
	ethernets:
	  enp2s0:
		dhcp4: true`
		if len(routes) > 0 {
			content = content + "\n      routes:"
			for _, route := range routes {
				content = content + fmt.Sprintf("\n      - to: %s", route)
				content = content + fmt.Sprintf("\n        via: %s", gateway)
			}
		}

		wf := writeFiles{
			Content: content,
			Path:    "/etc/netplan/intf.yaml",
		}

		ci.WriteFiles = append(ci.WriteFiles, wf)

		ci.Apt = apt{
			Primary: []primary{{
				Arches: []string{"default"},
				Uri:    "https://svl-artifactory.juniper.net/artifactory/common-ubuntu-remote/",
			}},
		}
		ci.RunCMD = append(ci.RunCMD, "netplan apply")
	}

	if registry != "" {
		content := `[[registry]]
location = "` + registry + `"
insecure = true`
		wf := writeFiles{
			Content: content,
			Path:    "/etc/containers/registries.conf.d/001-local.conf",
		}

		ci.WriteFiles = append(ci.WriteFiles, wf)
	}

	if criomirror != "" {
		runcmd := fmt.Sprintf(`echo "%s download.opensuse.org" >> /etc/hosts`, criomirror)
		ci.RunCMD = append(ci.RunCMD, runcmd)
	}

	ciByte, err := yaml.Marshal(&ci)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("#cloud-config\n%s", string(ciByte)), nil
}
