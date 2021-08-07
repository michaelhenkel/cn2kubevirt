package inventory

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/michaelhenkel/cn2kubevirt/cluster"
	"github.com/michaelhenkel/cn2kubevirt/roles"
	"gopkg.in/yaml.v3"
	"k8s.io/klog"
)

type Host struct {
	AnsibleHost string `yaml:"ansible_host"`
	IP          string `yaml:"ip"`
}

type All struct {
	Hosts map[string]Host   `yaml:"hosts"`
	Vars  map[string]string `yaml:"vars"`
}

type KubeMaster struct {
	Hosts map[string]struct{}
}

type KubeNode struct {
	Hosts map[string]struct{}
}

type Etcd struct {
	Hosts map[string]struct{}
}

type K8SCluster struct {
	Children map[string]struct{} `yaml:"children"`
}

type Inventory struct {
	All        All        `yaml:"all"`
	KubeMaster KubeMaster `yaml:"kube-master"`
	KubeNode   KubeNode   `yaml:"kube-node"`
	Etcd       Etcd       `yaml:"etcd"`
	K8SCluster K8SCluster `yaml:"k8s-cluster"`
}

type InstanceIPRole struct {
	Role     roles.Role
	Networks []roles.NetworkAnnotation
}

func NewInventory(instanceMap map[string]InstanceIPRole, cl cluster.Cluster, serviceIP string) error {
	var allHosts = make(map[string]Host)
	var kubeMasterHosts = make(map[string]struct{})
	var kubeNodeHosts = make(map[string]struct{})
	var etcdHosts = make(map[string]struct{})

	for instName, inst := range instanceMap {
		var ansibleHost string
		var ip string
		for _, nw := range inst.Networks {
			if nw.Name == fmt.Sprintf("%s/%s", cl.Namespace, cl.Name) {
				ip = nw.Ips[0]
			} else {
				ansibleHost = nw.Ips[0]
			}
		}
		allHosts[instName] = Host{
			AnsibleHost: ansibleHost,
			IP:          ip,
		}
		switch inst.Role {
		case roles.Controller:
			kubeMasterHosts[instName] = struct{}{}
			etcdHosts[instName] = struct{}{}

		case roles.Worker:
			kubeNodeHosts[instName] = struct{}{}
		}

	}
	i := Inventory{
		All: All{
			Hosts: allHosts,
			Vars: map[string]string{
				"enable_nodelocaldns":                 "false",
				"download_run_once":                   "true",
				"download_localhost":                  "true",
				"enable_dual_stack_networks":          "true",
				"ansible_user":                        "root",
				"docker_image_repo":                   "svl-artifactory.juniper.net/atom-docker-remote",
				"cluster_name":                        fmt.Sprintf("%s.%s", cl.Name, cl.Suffix),
				"artifacts_dir":                       cl.Kubeconfigdir,
				"kube_network_plugin":                 "cni",
				"kube_network_plugin_multus":          "false",
				"kubectl_localhost":                   "true",
				"kubeconfig_localhost":                "true",
				"override_system_hostname":            "true",
				"container_manager":                   "crio",
				"kubelet_deployment_type":             "host",
				"download_container":                  "false",
				"etcd_deployment_type":                "host",
				"host_key_checking":                   "false",
				"supplementary_addresses_in_ssl_keys": serviceIP,
			},
		},
		KubeMaster: KubeMaster{
			Hosts: kubeMasterHosts,
		},
		KubeNode: KubeNode{
			Hosts: kubeNodeHosts,
		},
		Etcd: Etcd{
			Hosts: etcdHosts,
		},
		K8SCluster: K8SCluster{
			Children: map[string]struct{}{
				"kube-master": struct{}{},
				"kube-node":   struct{}{},
			},
		},
	}
	inventoryByte, err := yaml.Marshal(&i)
	if err != nil {
		return err
	}
	inventoryString := strings.Replace(string(inventoryByte), "{}", "", -1)
	inventoryString = regexp.MustCompile(`"(true|false)"`).ReplaceAllString(inventoryString, `$1`)
	if _, err := os.Stat(cl.Kubeconfigdir); os.IsNotExist(err) {
		if err := os.Mkdir(cl.Kubeconfigdir, 0755); err != nil {
			return err
		}
	}

	if err := os.WriteFile(cl.Kubeconfigdir+"/inventory.yaml", []byte(inventoryString), 0600); err != nil {
		return err
	}
	klog.Infof("created inventory file %s/inventory.yaml", cl.Kubeconfigdir)

	return nil
}
