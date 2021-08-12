package kubevirt

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"

	"github.com/michaelhenkel/cn2kubevirt/cloudinit"
	"github.com/michaelhenkel/cn2kubevirt/cluster"
	"github.com/michaelhenkel/cn2kubevirt/inventory"
	"github.com/michaelhenkel/cn2kubevirt/k8s"
	"github.com/michaelhenkel/cn2kubevirt/roles"
	hd "github.com/mitchellh/go-homedir"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog"
	kubevirtV1 "kubevirt.io/client-go/api/v1"
	"kubevirt.io/client-go/kubecli"
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

func (k *KubevirtCluster) Create(client kubecli.KubevirtClient) error {
	for _, vmi := range k.VirtualMachineInstances {
		_, err := client.VirtualMachineInstance(vmi.Namespace).Get(vmi.Name, &metav1.GetOptions{})
		if errors.IsNotFound(err) {
			_, err := client.VirtualMachineInstance(vmi.Namespace).Create(vmi)
			if err != nil {
				return err
			}
		} else if err != nil {
			return err
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
		var done = make(chan bool)
		go func() {
			for event := range watch.ResultChan() {
				p, ok := event.Object.(*v1.Pod)
				if !ok {
					klog.Fatal("unexpected type")
				}
				/*
					klog.Infof("Type: %v\n", event.Type)
					klog.Infof("Status: %v\n", p.Status.ContainerStatuses)
					klog.Infof("Phase: %v\n", p.Status.Phase)
					klog.Infof("podname %s\n", p.Name)
					klog.Infof("ready %d, instances %d", readyCount, len(k.VirtualMachineInstances))
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

func NewKubevirtCluster(cl *cluster.Cluster) (*KubevirtCluster, error) {
	kvCluster := &KubevirtCluster{}
	expandedKeypath, err := hd.Expand(cl.Keypath)
	if err != nil {
		return nil, err
	}
	pubKey, err := ioutil.ReadFile(expandedKeypath)
	if err != nil {
		return nil, err
	}
	for c := 0; c < cl.Controller; c++ {
		ci, err := cloudinit.CreateCloudInit(fmt.Sprintf("%s-%d", roles.Controller, c), string(pubKey))
		if err != nil {
			return nil, err
		}
		kvCluster.VirtualMachineInstances = append(kvCluster.VirtualMachineInstances, defineVMI(cl, ci, c, roles.Controller))
	}
	for c := 0; c < cl.Worker; c++ {
		ci, err := cloudinit.CreateCloudInit(fmt.Sprintf("%s-%d", roles.Worker, c), string(pubKey))
		if err != nil {
			return nil, err
		}
		kvCluster.VirtualMachineInstances = append(kvCluster.VirtualMachineInstances, defineVMI(cl, ci, c, roles.Worker))
	}
	/*
		clByte, err := yaml.Marshal(kvCluster)
		if err != nil {
			return nil, err
		}
		fmt.Println(string(clByte))
	*/
	return kvCluster, nil
}

func defineVMI(cl *cluster.Cluster, ci string, idx int, role roles.Role) *kubevirtV1.VirtualMachineInstance {
	return &kubevirtV1.VirtualMachineInstance{
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
						Image:           cl.Image,
						ImagePullPolicy: "Always",
					},
				},
			}, {
				Name: "cloudinitdisk",
				VolumeSource: kubevirtV1.VolumeSource{
					CloudInitNoCloud: &kubevirtV1.CloudInitNoCloudSource{
						UserData: ci,
					},
				},
			}},
		},
	}

}
