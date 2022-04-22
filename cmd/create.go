package cmd

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"time"

	"github.com/michaelhenkel/cn2kubevirt/cluster"
	"github.com/michaelhenkel/cn2kubevirt/inventory"
	"github.com/michaelhenkel/cn2kubevirt/k8s"
	"github.com/michaelhenkel/cn2kubevirt/kubevirt"
	"github.com/michaelhenkel/cn2kubevirt/roles"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
	v1 "k8s.io/api/core/v1"

	nadv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func init() {
	createCmd.PersistentFlags().StringVarP(&file, "file", "f", "", "file")
}

var createCmd = &cobra.Command{
	Use:   "create",
	Short: "",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		if file != "" {
			log.Info("creating cluster")
			if err := createCluster(); err != nil {
				log.Error(err)
				os.Exit(0)
			}
		} else {
			log.Errorf("missing file")
			os.Exit(0)
		}
	},
}

func createNad(cl *cluster.Cluster, client *k8s.Client, subnet string, name string, snat bool) error {
	_, err := client.Nad.K8sCniCncfIoV1().NetworkAttachmentDefinitions(cl.Namespace).Get(context.Background(), name, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		/*
			var networkDef string
			if snat {
				networkDef = fmt.Sprintf(`{"ipamV4Subnet": "%s","fabricSNAT": true}`, subnet)
			} else {
				networkDef = fmt.Sprintf(`{"ipamV4Subnet": "%s"}`, subnet)
			}
		*/
		networkDef := fmt.Sprintf(`{"ipamV4Subnet": "%s"}`, subnet)
		nad := &nadv1.NetworkAttachmentDefinition{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: cl.Namespace,
				Annotations: map[string]string{
					"juniper.net/networks": networkDef,
				},
			},
			Spec: nadv1.NetworkAttachmentDefinitionSpec{
				Config: `{"cniVersion": "0.3.1","name": "contrail-k8s-cni",	"type": "contrail-k8s-cni"}`,
			},
		}
		log.Infof("Creating NAD %s", name)
		_, err = client.Nad.K8sCniCncfIoV1().NetworkAttachmentDefinitions(cl.Namespace).Create(context.Background(), nad, metav1.CreateOptions{})
		if err != nil {
			return err
		}

		watch := false
		nad, err = client.Nad.K8sCniCncfIoV1().NetworkAttachmentDefinitions(cl.Namespace).Get(context.Background(), nad.Name, metav1.GetOptions{})
		if errors.IsNotFound(err) {
			watch = true
		} else if err != nil {
			return err
		}
		txt := fmt.Sprintf("success creating VirtualNetwork %s v4Subnet: %s ", name, subnet)
		status, ok := nad.ObjectMeta.Annotations["juniper.net/networks-status"]
		if !ok || status != txt {
			watch = true
		}
		if watch {
			log.Infof("Waiting for NAD %s", name)
			wait := 5
			for i := 0; i < wait; i++ {
				nad, err = client.Nad.K8sCniCncfIoV1().NetworkAttachmentDefinitions(cl.Namespace).Get(context.Background(), nad.Name, metav1.GetOptions{})
				if err == nil {
					txt := fmt.Sprintf("success creating VirtualNetwork %s v4Subnet: %s ", name, subnet)
					status, ok := nad.ObjectMeta.Annotations["juniper.net/networks-status"]
					if ok && status == txt {
						break
					}
				}
				time.Sleep(time.Second * 1)
			}
		}
		log.Infof("NAD created %s", name)
	} else if err != nil {
		return err
	} else {
		log.Infof("NAD %s already exists", subnet)
	}
	vn, err := client.Contrail.CoreV1alpha1().VirtualNetworks(cl.Namespace).Get(context.Background(), name, metav1.GetOptions{})
	if err != nil {
		return err
	}
	log.Info("Adding label to VirtualNetwork")
	vn.Labels["core.juniper.net/virtualnetwork"] = "cluster"
	if _, err := client.Contrail.CoreV1alpha1().VirtualNetworks(cl.Namespace).Update(context.Background(), vn, metav1.UpdateOptions{}); err != nil {
		return err
	}
	return nil
}

func createCluster() error {
	clusterByte, err := ioutil.ReadFile(file)
	if err != nil {
		return err
	}
	cl := &cluster.Cluster{}
	if err := yaml.Unmarshal(clusterByte, cl); err != nil {
		return err
	}
	client, err := k8s.NewClient()
	if err != nil {
		return err
	}
	_, err = client.K8S.CoreV1().Namespaces().Get(context.Background(), cl.Namespace, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		namespace := &v1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name:   cl.Namespace,
				Labels: map[string]string{"namespace": "cluster"},
			},
		}
		log.Infof("Creating namespace %s", cl.Namespace)
		_, err = client.K8S.CoreV1().Namespaces().Create(context.Background(), namespace, metav1.CreateOptions{})
		if err != nil {
			return err
		}
	} else if err != nil {
		return err
	} else {
		log.Infof("Namespace %s already exists", cl.Namespace)
	}
	if err := createNad(cl, client, cl.Subnet, cl.Name, true); err != nil {
		return err
	}
	//fmt.Println("CTRL", cl.Ctrldatasubnet)
	//os.Exit(0)
	if cl.Ctrldatasubnet != "" {
		if err := createNad(cl, client, cl.Ctrldatasubnet, cl.Name+"-ctrldata", false); err != nil {
			return err
		}
	}

	kvc, err := kubevirt.NewKubevirtCluster(cl, client)
	if err != nil {
		return err
	}
	if err := kvc.Create(client.Kubevirt); err != nil {
		return err
	}
	instanceMap, err := kvc.Watch(client, cl)
	if err != nil {
		return err
	}
	var serviceIP string
	watch := false
	svc, err := client.K8S.CoreV1().Services(cl.Namespace).Get(context.Background(), cl.Name, metav1.GetOptions{})
	if err == nil {
		if svc.Spec.ClusterIP != "" {
			serviceIP = svc.Spec.ClusterIP
		} else {
			watch = true
		}
	} else if errors.IsNotFound(err) {
		service := &v1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      cl.Name,
				Namespace: cl.Namespace,
				Labels:    map[string]string{"cluster": cl.Name},
			},
			Spec: v1.ServiceSpec{
				Ports: []v1.ServicePort{{
					Name: "api",
					Port: 6443,
					TargetPort: intstr.IntOrString{
						IntVal: 6443,
					},
					Protocol: v1.ProtocolTCP,
				}},
				Selector: map[string]string{
					"cluster": cl.Name,
					"role":    string(roles.Controller),
				},
			},
		}
		log.Infof("Creating Service %s", cl.Name)
		if _, err := client.K8S.CoreV1().Services(cl.Namespace).Create(context.Background(), service, metav1.CreateOptions{}); err != nil {
			return err
		}
		watch = true

	} else if err != nil {
		return err
	} else {
		log.Infof("Service %s already exists", cl.Name)
	}
	if watch {
		log.Infof("Waiting for ClusterIP")
		watch, err := client.K8S.CoreV1().Services(cl.Namespace).Watch(context.Background(), metav1.ListOptions{
			LabelSelector: fmt.Sprintf("cluster=%s", cl.Name),
		})
		if err != nil {
			return err
		}

		var done = make(chan bool)
		go func() {
			for event := range watch.ResultChan() {
				s, ok := event.Object.(*v1.Service)
				if !ok {
					log.Fatal("unexpected type")
				}
				if s.Spec.ClusterIP != "" {
					serviceIP = s.Spec.ClusterIP
					done <- true
				}

			}
		}()
		<-done
	}
	log.Infof("ClusterIP: %s", serviceIP)

	registrySvc := "svl-artifactory.juniper.net/atom-docker/cn2/bazel-build/dev"
	_, err = client.K8S.CoreV1().Services("default").Get(context.Background(), "registry", metav1.GetOptions{})
	if err == nil {
		registrySvc = "registry.default.svc.cluster1.local:5000"
	}
	var ctrlData bool
	if cl.Ctrldatasubnet != "" {
		ctrlData = true
	}
	if err := inventory.NewInventory(instanceMap, *cl, serviceIP, registrySvc, ctrlData); err != nil {
		return err
	}
	return nil
}
