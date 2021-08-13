package k8s

import (
	"path/filepath"

	nadClientset "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/client/clientset/versioned"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
	kvClientset "kubevirt.io/client-go/kubecli"
	contrailClient "ssd-git.juniper.net/contrail/cn2/contrail/pkg/client/clientset_generated/clientset"
)

type Client struct {
	K8S      *kubernetes.Clientset
	Kubevirt kvClientset.KubevirtClient
	Nad      *nadClientset.Clientset
	Contrail contrailClient.Clientset
}

func NewClient() (*Client, error) {
	var kubeconfig string
	if home := homedir.HomeDir(); home != "" {
		kubeconfig = filepath.Join(home, ".kube", "config")
	}
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return nil, err
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}
	kubevirtClient, err := kvClientset.GetKubevirtClientFromRESTConfig(config)
	if err != nil {
		return nil, err
	}
	nadClient, err := nadClientset.NewForConfig(config)
	if err != nil {
		return nil, err
	}
	contrailClient, err := contrailClient.NewForConfig(config)
	if err != nil {
		return nil, err
	}
	return &Client{
		K8S:      clientset,
		Kubevirt: kubevirtClient,
		Nad:      nadClient,
		Contrail: *contrailClient,
	}, nil
}
