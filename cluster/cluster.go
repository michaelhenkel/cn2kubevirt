package cluster

type Cluster struct {
	Name            string
	Namespace       string
	Controller      int
	Worker          int
	Subnet          string
	Keypath         string
	Memory          string
	Cpu             string
	Image           string
	Suffix          string
	Kubeconfigdir   string
	Podv4subnet     string
	Podv6subnet     string
	Servicev4subnet string
	Servicev6subnet string
	Asn             int
}
