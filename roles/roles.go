package roles

type Role string

const (
	Worker     Role = "worker"
	Controller Role = "controller"
)

type NetworkAnnotation struct {
	Name      string
	Interface string
	Ips       []string
}
