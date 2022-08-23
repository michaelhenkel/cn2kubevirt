package kubevirt

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"

	"github.com/michaelhenkel/cn2kubevirt/cloudinit"
	"github.com/michaelhenkel/cn2kubevirt/cluster"
	"github.com/michaelhenkel/cn2kubevirt/inventory"
	"github.com/michaelhenkel/cn2kubevirt/k8s"
	"github.com/michaelhenkel/cn2kubevirt/roles"
	hd "github.com/mitchellh/go-homedir"
	log "github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubevirtV1 "kubevirt.io/api/core/v1"
	kvClientset "kubevirt.io/client-go/kubecli"
)

type KubevirtCluster struct {
	VirtualMachineInstances []*kubevirtV1.VirtualMachineInstance
}

type Node struct {
	VirtualMachineInstance *kubevirtV1.VirtualMachineInstance
}

type NetworkAnnotation struct {
	Name      string
	Interface string
	Ips       []string
}

func (k *KubevirtCluster) Create(client kvClientset.KubevirtClient) error {

	for _, vmi := range k.VirtualMachineInstances {
		_, err := client.VirtualMachineInstance(vmi.Namespace).Get(vmi.Name, &metav1.GetOptions{})
		if errors.IsNotFound(err) {
			log.Infof("Creating VMI %s", vmi.Name)
			_, err := client.VirtualMachineInstance(vmi.Namespace).Create(vmi)
			if err != nil {
				return err
			}
		} else if err != nil {
			return err
		} else {
			log.Infof("VMI %s already exists", vmi.Name)
		}
	}
	return nil
}

func (k *KubevirtCluster) Watch(client *k8s.Client, cl *cluster.Cluster) (map[string]inventory.InstanceIPRole, error) {
	readyCount := 0
	podList, err := client.K8S.CoreV1().Pods(cl.Namespace).List(context.Background(), metav1.ListOptions{
		LabelSelector: fmt.Sprintf("cluster=%s", cl.Name),
	})
	if err != nil {
		return nil, err
	}
	for _, pod := range podList.Items {
		if pod.Status.Phase == "Running" {
			readyCount++
		}
	}
	if readyCount != len(k.VirtualMachineInstances) {
		readyCount = 0
		watch, err := client.K8S.CoreV1().Pods(cl.Namespace).Watch(context.Background(), metav1.ListOptions{
			LabelSelector: fmt.Sprintf("cluster=%s", cl.Name),
		})
		if err != nil {
			return nil, err
		}
		log.Infof("Waiting for VMIs")
		var done = make(chan bool)
		go func() {
			for event := range watch.ResultChan() {
				p, ok := event.Object.(*v1.Pod)
				if !ok {
					log.Fatal("unexpected type")
				}
				/*
					log.Infof("Type: %v\n", event.Type)
					log.Infof("Status: %v\n", p.Status.ContainerStatuses)
					log.Infof("Phase: %v\n", p.Status.Phase)
					log.Infof("podname %s\n", p.Name)
					log.Infof("ready %d, instances %d", readyCount, len(k.VirtualMachineInstances))
				*/
				if p.Status.Phase == "Running" {
					readyCount++
				}
				if readyCount == len(k.VirtualMachineInstances) {
					done <- true
				}

			}
		}()
		<-done
		log.Infof("VMIs are up and running")
	}
	newPodList, err := client.K8S.CoreV1().Pods(cl.Namespace).List(context.Background(), metav1.ListOptions{
		LabelSelector: fmt.Sprintf("cluster=%s", cl.Name),
	})
	if err != nil {
		return nil, err
	}
	var instanceMap = make(map[string]inventory.InstanceIPRole)
	for _, pod := range newPodList.Items {
		var networkAnnotationList []roles.NetworkAnnotation
		networkAnnotationString, ok := pod.Annotations["k8s.v1.cni.cncf.io/network-status"]
		if !ok {
			return nil, fmt.Errorf("no network annotation")
		}
		if err := json.Unmarshal([]byte(networkAnnotationString), &networkAnnotationList); err != nil {
			return nil, err
		}
		instanceMap[pod.Spec.Hostname] = inventory.InstanceIPRole{
			Role:     roles.Role(pod.Labels["role"]),
			Networks: networkAnnotationList,
		}
	}

	return instanceMap, nil
}

func NewKubevirtCluster(cl *cluster.Cluster, client *k8s.Client) (*KubevirtCluster, error) {
	var criomirror string
	aptSvc, err := client.K8S.CoreV1().Services("default").Get(context.Background(), "aptmirror", metav1.GetOptions{})
	if err != nil && !errors.IsNotFound(err) {
		return nil, err
	} else if err == nil {
		criomirror = aptSvc.Spec.ClusterIP
	}
	dnsSvc, err := client.K8S.CoreV1().Services("kube-system").Get(context.Background(), "coredns", metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	dnsSvcIP := dnsSvc.Spec.ClusterIP

	var registrySvc string

	image := fmt.Sprintf("svl-artifactory.juniper.net/atom-docker/cn2/bazel-build/dev/%s", cl.Image)

	_, err = client.K8S.CoreV1().Services("default").Get(context.Background(), "registry", metav1.GetOptions{})
	if err == nil {
		registrySvc = "registry.default.svc.cluster1.local:5000"
		//image = fmt.Sprintf("%s:5000/%s", regSvc.Spec.ClusterIP, cl.Image)
		image = fmt.Sprintf("%s/%s", registrySvc, cl.Image)
	}

	kvCluster := &KubevirtCluster{}
	expandedKeypath, err := hd.Expand(cl.Keypath)
	if err != nil {
		return nil, err
	}
	pubKey, err := ioutil.ReadFile(expandedKeypath)
	if err != nil {
		return nil, err
	}
	ipnet, _, err := net.ParseCIDR(cl.Subnet)
	if err != nil {
		return nil, err
	}
	ip := ipnet.To4()
	ip[3]++

	var ctrlData bool
	if cl.Ctrldatasubnet != "" {
		ctrlData = true
	}
	for c := 0; c < cl.Controller; c++ {

		ci, err := cloudinit.CreateCloudInit(fmt.Sprintf("%s-%d", roles.Controller, c), string(pubKey), ip.String(), criomirror, dnsSvcIP, registrySvc, cl.Routes, cl.Distro, ctrlData)
		if err != nil {
			return nil, err
		}
		secret := &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("%s-%d", roles.Controller, c),
				Namespace: cl.Namespace,
			},
			StringData: map[string]string{"userdata": ci},
		}
		if _, err := client.K8S.CoreV1().Secrets(cl.Namespace).Create(context.Background(), secret, metav1.CreateOptions{}); err != nil {
			if errors.IsAlreadyExists(err) {
				if _, err := client.K8S.CoreV1().Secrets(cl.Namespace).Update(context.Background(), secret, metav1.UpdateOptions{}); err != nil {
					return nil, err
				}
			} else {
				return nil, err
			}
		}
		kvCluster.VirtualMachineInstances = append(kvCluster.VirtualMachineInstances, defineVMI(cl, ci, c, roles.Controller, image))
	}
	for c := 0; c < cl.Worker; c++ {
		ci, err := cloudinit.CreateCloudInit(fmt.Sprintf("%s-%d", roles.Worker, c), string(pubKey), ip.String(), criomirror, dnsSvcIP, registrySvc, cl.Routes, cl.Distro, ctrlData)
		if err != nil {
			return nil, err
		}
		secret := &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("%s-%d", roles.Worker, c),
				Namespace: cl.Namespace,
			},
			StringData: map[string]string{"userdata": ci},
		}
		if _, err := client.K8S.CoreV1().Secrets(cl.Namespace).Create(context.Background(), secret, metav1.CreateOptions{}); err != nil {
			if errors.IsAlreadyExists(err) {
				if _, err := client.K8S.CoreV1().Secrets(cl.Namespace).Update(context.Background(), secret, metav1.UpdateOptions{}); err != nil {
					return nil, err
				}
			} else {
				return nil, err
			}
		}
		kvCluster.VirtualMachineInstances = append(kvCluster.VirtualMachineInstances, defineVMI(cl, ci, c, roles.Worker, image))
	}
	return kvCluster, nil
}

func defineVMI(cl *cluster.Cluster, ci string, idx int, role roles.Role, image string) *kubevirtV1.VirtualMachineInstance {
	vmi := &kubevirtV1.VirtualMachineInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-%d", role, idx),
			Namespace: cl.Namespace,
			Labels:    map[string]string{"cluster": cl.Name, "role": string(role)},
		},
		Spec: kubevirtV1.VirtualMachineInstanceSpec{
			Networks: []kubevirtV1.Network{{
				Name: "default",
				NetworkSource: kubevirtV1.NetworkSource{
					Pod: &kubevirtV1.PodNetwork{},
				},
			}, {
				Name: cl.Name,
				NetworkSource: kubevirtV1.NetworkSource{
					Multus: &kubevirtV1.MultusNetwork{
						NetworkName: fmt.Sprintf("%s/%s", cl.Namespace, cl.Name),
					},
				},
			}},
			Domain: kubevirtV1.DomainSpec{
				Resources: kubevirtV1.ResourceRequirements{
					Requests: v1.ResourceList{
						"memory": resource.MustParse(cl.Memory),
						"cpu":    resource.MustParse(cl.Cpu),
					},
				},
				Devices: kubevirtV1.Devices{
					Interfaces: []kubevirtV1.Interface{{
						Name: "default",
						InterfaceBindingMethod: kubevirtV1.InterfaceBindingMethod{
							Bridge: &kubevirtV1.InterfaceBridge{},
						},
					}, {
						Name: cl.Name,
						InterfaceBindingMethod: kubevirtV1.InterfaceBindingMethod{
							Bridge: &kubevirtV1.InterfaceBridge{},
						},
					}},
					Disks: []kubevirtV1.Disk{{
						Name: fmt.Sprintf("%s-disk", cl.Name),
						DiskDevice: kubevirtV1.DiskDevice{
							Disk: &kubevirtV1.DiskTarget{
								Bus: "virtio",
							},
						},
					}, {
						Name: "cloudinitdisk",
						DiskDevice: kubevirtV1.DiskDevice{
							Disk: &kubevirtV1.DiskTarget{
								Bus: "virtio",
							},
						},
					}},
				},
			},
			Volumes: []kubevirtV1.Volume{{
				Name: fmt.Sprintf("%s-disk", cl.Name),
				VolumeSource: kubevirtV1.VolumeSource{
					ContainerDisk: &kubevirtV1.ContainerDiskSource{
						Image:           image,
						ImagePullPolicy: "Always",
					},
				},
			}, {
				Name: "cloudinitdisk",
				VolumeSource: kubevirtV1.VolumeSource{
					CloudInitNoCloud: &kubevirtV1.CloudInitNoCloudSource{
						UserDataSecretRef: &v1.LocalObjectReference{
							Name: fmt.Sprintf("%s-%d", role, idx),
						},
					},
				},
			}},
		},
	}
	if cl.Ctrldatasubnet != "" {
		vmi.Spec.Networks = append(vmi.Spec.Networks, kubevirtV1.Network{
			Name: cl.Name + "-ctrldata",
			NetworkSource: kubevirtV1.NetworkSource{
				Multus: &kubevirtV1.MultusNetwork{
					NetworkName: fmt.Sprintf("%s/%s", cl.Namespace, cl.Name+"-ctrldata"),
				},
			},
		})
		vmi.Spec.Domain.Devices.Interfaces = append(vmi.Spec.Domain.Devices.Interfaces, kubevirtV1.Interface{
			Name: cl.Name + "-ctrldata",
			InterfaceBindingMethod: kubevirtV1.InterfaceBindingMethod{
				Bridge: &kubevirtV1.InterfaceBridge{},
			},
		})
	}
	return vmi

}
