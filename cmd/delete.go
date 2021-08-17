package cmd

import (
	"context"
	"os"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/michaelhenkel/cn2kubevirt/k8s"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func init() {

}

var deleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) == 1 {
			if err := delete(args[0]); err != nil {
				log.Error(err)
				os.Exit(0)
			}
		} else {
			log.Errorf("cluster name is missing")
			os.Exit(0)
		}
	},
}

func delete(clusterName string) error {
	client, err := k8s.NewClient()
	if err != nil {
		return err
	}
	vmiList, err := client.Kubevirt.VirtualMachineInstance(clusterName).List(&metav1.ListOptions{})
	if err != nil {
		return err
	}
	for _, vmi := range vmiList.Items {
		if err := client.Kubevirt.VirtualMachineInstance(clusterName).Delete(vmi.Name, &metav1.DeleteOptions{}); err != nil {
			return err
		}
	}
	nadList, err := client.Nad.K8sCniCncfIoV1().NetworkAttachmentDefinitions(clusterName).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return err
	}
	for _, nad := range nadList.Items {
		if err := client.Nad.K8sCniCncfIoV1().NetworkAttachmentDefinitions(clusterName).Delete(context.Background(), nad.Name, metav1.DeleteOptions{}); err != nil {
			return err
		}
	}
	svcList, err := client.K8S.CoreV1().Services(clusterName).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return err
	}
	for _, svc := range svcList.Items {
		if err := client.K8S.CoreV1().Services(clusterName).Delete(context.Background(), svc.Name, metav1.DeleteOptions{}); err != nil {
			return err
		}
	}

	secretList, err := client.K8S.CoreV1().Secrets(clusterName).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return err
	}
	for _, secret := range secretList.Items {
		if err := client.K8S.CoreV1().Secrets(clusterName).Delete(context.Background(), secret.Name, metav1.DeleteOptions{}); err != nil {
			return err
		}
	}
	/*
		vnrList, err := client.Contrail.CoreV1alpha1().VirtualNetworkRouters(clusterName).List(context.Background(), metav1.ListOptions{})
		if err != nil {
			return err
		}

		for _, vnr := range vnrList.Items {
			if err := client.Contrail.CoreV1alpha1().VirtualNetworkRouters(clusterName).Delete(context.Background(), vnr.GetName(), metav1.DeleteOptions{}); err != nil {
				return err
			}
		}
	*/
	if err := client.K8S.CoreV1().Namespaces().Delete(context.Background(), clusterName, metav1.DeleteOptions{}); err != nil {
		return err
	}
	return nil
}
