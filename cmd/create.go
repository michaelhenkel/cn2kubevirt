package cmd

import (
	"fmt"
	"io/ioutil"
	"os"

	"github.com/michaelhenkel/cn2kubevirt/cluster"
	"github.com/michaelhenkel/cn2kubevirt/inventory"
	"github.com/michaelhenkel/cn2kubevirt/k8s"
	"github.com/michaelhenkel/cn2kubevirt/kubevirt"
	"github.com/michaelhenkel/cn2kubevirt/roles"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
	v1 "k8s.io/api/core/v1"
	"k8s.io/klog"

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
			klog.Info("creating cluster")
			if err := createCluster(); err != nil {
				klog.Error(err)
				os.Exit(0)
			}
		} else {
			klog.Errorf("missing file")
			os.Exit(0)
		}
	},
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
	_, err = client.K8S.CoreV1().Namespaces().Get(cl.Namespace, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		namespace := &v1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: cl.Namespace,
			},
		}
		_, err = client.K8S.CoreV1().Namespaces().Create(namespace)
		if err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	_, err = client.Nad.K8sCniCncfIoV1().NetworkAttachmentDefinitions(cl.Namespace).Get(cl.Name, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		nad := &nadv1.NetworkAttachmentDefinition{
			ObjectMeta: metav1.ObjectMeta{
				Name:      cl.Name,
				Namespace: cl.Namespace,
				Annotations: map[string]string{
					"juniper.net/networks": fmt.Sprintf(`{"ipamV4Subnet": "%s"}`, cl.Subnet),
				},
			},
			Spec: nadv1.NetworkAttachmentDefinitionSpec{
				Config: `{"cniVersion": "0.3.1","name": "contrail-k8s-cni",	"type": "contrail-k8s-cni"}`,
			},
		}
		_, err = client.Nad.K8sCniCncfIoV1().NetworkAttachmentDefinitions(cl.Namespace).Create(nad)
		if err != nil {
			return err
		}
	} else if err != nil {
		return err
	}

	kvc, err := kubevirt.NewKubevirtCluster(cl)
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
	svc, err := client.K8S.CoreV1().Services(cl.Namespace).Get(cl.Name, metav1.GetOptions{})
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
		if _, err := client.K8S.CoreV1().Services(cl.Namespace).Create(service); err != nil {
			return err
		}
		watch = true

	} else if err != nil {
		return err
	}
	if watch {
		watch, err := client.K8S.CoreV1().Services(cl.Namespace).Watch(metav1.ListOptions{
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
					klog.Fatal("unexpected type")
				}
				if s.Spec.ClusterIP != "" {
					serviceIP = s.Spec.ClusterIP
					done <- true
				}

			}
		}()
		<-done
	}
	if err := inventory.NewInventory(instanceMap, *cl, serviceIP); err != nil {
		return err
	}
	return nil
}
