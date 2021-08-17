package roles

type Role string
type Distro string

const (
	Worker     Role   = "worker"
	Controller Role   = "controller"
	Centos     Distro = "centos"
	Ubuntu     Distro = "ubuntu"
)

type NetworkAnnotation struct {
	Name      string
	Interface string
	Ips       []string
}
