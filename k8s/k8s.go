package k8s

import (
	"path/filepath"

	nadClientset "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/client/clientset/versioned"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
	"kubevirt.io/client-go/kubecli"
)

type Client struct {
	K8S      *kubernetes.Clientset
	Kubevirt kubecli.KubevirtClient
	Nad      *nadClientset.Clientset
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
	kubevirtClient, err := kubecli.GetKubevirtClientFromRESTConfig(config)
	if err != nil {
		return nil, err
	}
	nadClient, err := nadClientset.NewForConfig(config)
	return &Client{
		K8S:      clientset,
		Kubevirt: kubevirtClient,
		Nad:      nadClient,
	}, nil
}
