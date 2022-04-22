package inventory

import (
	"fmt"

	"net"
	"os"
	"regexp"
	"strings"

	"github.com/michaelhenkel/cn2kubevirt/cluster"
	"github.com/michaelhenkel/cn2kubevirt/deployer"
	"github.com/michaelhenkel/cn2kubevirt/roles"
	log "github.com/sirupsen/logrus"

	"gopkg.in/yaml.v3"
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

func NewInventory(instanceMap map[string]InstanceIPRole, cl cluster.Cluster, serviceIP, registry string, ctrlData bool) error {
	var allHosts = make(map[string]Host)
	var kubeMasterHosts = make(map[string]struct{})
	var kubeNodeHosts = make(map[string]struct{})
	var etcdHosts = make(map[string]struct{})
	name := cl.Name
	if ctrlData {
		name = cl.Name + "-ctrldata"
	}
	for instName, inst := range instanceMap {
		var ansibleHost string
		var ip string
		for _, nw := range inst.Networks {
			if nw.Name == fmt.Sprintf("%s/%s", cl.Namespace, name) {
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
				"supplementary_addresses_in_ssl_keys": "[" + serviceIP + "]",
				"kube_service_addresses":              cl.Servicev4subnet,
				"kube_pods_subnet":                    cl.Podv4subnet,
				"kube_service_addresses_ipv6":         cl.Servicev6subnet,
				"kube_pods_subnet_ipv6":               cl.Podv6subnet,
				//"loadbalancer_apiserver":              serviceIP,
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
	inventoryString = strings.Replace(inventoryString, "'[", "[", -1)
	inventoryString = strings.Replace(inventoryString, "]'", "]", -1)
	if _, err := os.Stat(cl.Kubeconfigdir); os.IsNotExist(err) {
		if err := os.Mkdir(cl.Kubeconfigdir, 0755); err != nil {
			return err
		}
	}

	if err := os.WriteFile(cl.Kubeconfigdir+"/inventory.yaml", []byte(inventoryString), 0600); err != nil {
		return err
	}
	log.Infof("created inventory file %s/inventory.yaml", cl.Kubeconfigdir)

	/*
		adminConfByte, err := os.ReadFile(cl.Kubeconfigdir + "/admin.conf")
		if err != nil {
			return err
		}
		r := regexp.MustCompile(`server: https://(.*):6443`)
		currentIP := r.FindStringSubmatch(string(adminConfByte))
		adminConfString := strings.Replace(string(string(adminConfByte)), currentIP[1], serviceIP, -1)
		if err := os.WriteFile(cl.Kubeconfigdir+"/admin.conf", []byte(adminConfString), 0600); err != nil {
			return err
		}
	*/
	log.Infof("created kubeconfig file %s/admin.conf", cl.Kubeconfigdir)
	ipnet, _, err := net.ParseCIDR(cl.Subnet)
	if err != nil {
		return err
	}
	ip := ipnet.To4()
	ip[3]++
	deployer := deployer.NewDeployer(cl.Controller, ip.String(), cl.Podv4subnet, cl.Podv6subnet, cl.Servicev4subnet, cl.Servicev6subnet, registry, cl.Tag, cl.Asn)
	if err := os.WriteFile(cl.Kubeconfigdir+"/deployer.yaml", []byte(deployer), 0600); err != nil {
		return err
	}
	log.Infof("created deployer file %s/deployer.yaml", cl.Kubeconfigdir)
	return nil
}
