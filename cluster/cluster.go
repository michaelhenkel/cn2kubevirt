package cluster

type Cluster struct {
	Name          string
	Namespace     string
	Controller    int
	Worker        int
	Subnet        string
	Keypath       string
	Memory        string
	Cpu           string
	Image         string
	Suffix        string
	Kubeconfigdir string
}
