module github.com/michaelhenkel/cn2kubevirt

go 1.16

require (
	github.com/k8snetworkplumbingwg/network-attachment-definition-client v0.0.0-20181121151021-386d141f4c94
	//github.com/k8snetworkplumbingwg/network-attachment-definition-client v1.1.0 // indirect
	github.com/mitchellh/go-homedir v1.1.0
	github.com/spf13/cobra v1.2.1
	gopkg.in/yaml.v3 v3.0.0-20210107192922-496545a6307b
	k8s.io/api v0.0.0-20190222213804-5cb15d344471
	k8s.io/apimachinery v0.20.2
	k8s.io/client-go v0.0.0-20190228174230-b40b2a5939e4
	//k8s.io/client-go v0.18.3
	//k8s.io/client-go v0.21.0 // indirect
	k8s.io/klog v1.0.0
	kubevirt.io/client-go v0.19.0
)

replace k8s.io/apimachinery => k8s.io/apimachinery v0.0.0-20190221213512-86fb29eff628
