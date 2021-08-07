package cloudinit

import (
	"fmt"

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
	APT            map[string]source `yaml:"apt"`
	Snap           map[string]string `yaml:"snap"`
	Network        netw              `yaml:"network"`
}

type netw struct {
	Version   string              `yaml:"version"`
	Ethernets map[string]ethernet `yaml:"ethernets"`
}

type ethernet struct {
	Match map[string]string `yaml:"match"`
	Dhcp4 bool              `yaml:"dhcp4"`
}

type source struct {
	Source string `yaml:"source"`
	KeyID  string `yaml:"keyid"`
}

type chpasswd struct {
	List   string `yaml:"list"`
	Expire bool   `yaml:"expire"`
}

type writeFiles struct {
	Content string `yaml:"content"`
	Path    string `yaml:"path"`
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

func CreateCloudInit(hostname, key string) (string, error) {
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
DNS=172.29.131.60`,
			Path: "/etc/systemd/resolved.conf",
		}, {
			Content: `network:
  ethernets:
    enp2s0:
      dhcp4: true`,
			Path: "/etc/netplan/intf.yaml",
		}},
		RunCMD: []string{
			"systemctl restart systemd-resolved.service",
			"netplan apply",
		},
	}

	ciByte, err := yaml.Marshal(&ci)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("#cloud-config\n%s", string(ciByte)), nil
}
