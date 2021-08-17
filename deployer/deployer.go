package deployer

import (
	"strconv"
	"strings"
)

var deployerTemplate = `apiVersion: v1
kind: Namespace
metadata:
  name: contrail
---
apiVersion: v1
kind: Namespace
metadata:
  name: contrail-deploy
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: contrail-serviceaccount
  namespace: contrail
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: contrail-deploy-serviceaccount
  namespace: contrail-deploy
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: contrail-role
rules:
- apiGroups:
  - '*'
  resources:
  - '*'
  verbs:
  - '*'
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: contrail-deploy-role
rules:
- apiGroups:
  - '*'
  resources:
  - '*'
  verbs:
  - '*'
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: contrail-rolebinding
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: contrail-role
subjects:
- kind: ServiceAccount
  name: contrail-serviceaccount
  namespace: contrail
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: contrail-deploy-rolebinding
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: contrail-deploy-role
subjects:
- kind: ServiceAccount
  name: contrail-deploy-serviceaccount
  namespace: contrail-deploy
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: contrail-k8s-deployer
  namespace: contrail-deploy
spec:
  replicas: 1
  selector:
    matchLabels:
      app: contrail-k8s-deployer
  template:
    metadata:
      labels:
        app: contrail-k8s-deployer
    spec:
      containers:
      - command:
        - sh
        - -c
        - /manager --metrics-addr 127.0.0.1:8081
        image: REGISTRY/contrail-k8s-deployer:TAG
        imagePullPolicy: Always
        name: contrail-k8s-deployer
      hostNetwork: true
      initContainers:
      - command:
        - sh
        - -c
        - kustomize build /crd | kubectl apply -f -
        image: REGISTRY/contrail-k8s-crdloader:TAG
        imagePullPolicy: Always
        name: contrail-k8s-crdloader
      nodeSelector:
        node-role.kubernetes.io/master: ""
      serviceAccountName: contrail-deploy-serviceaccount
      tolerations:
      - effect: NoSchedule
        operator: Exists
      - effect: NoExecute
        operator: Exists
---
apiVersion: v1
data:
  contrail-cr.yaml: |
    apiVersion: configplane.juniper.net/v1alpha1
    kind: ApiServer
    metadata:
      name: contrail-k8s-apiserver
      namespace: contrail
    spec:
      autonomousSystem: ASN
      common:
        replicas: REPLICAS
        containers:
        - image: REGISTRY/contrail-k8s-apiserver:TAG
          name: contrail-k8s-apiserver
        nodeSelector:
          node-role.kubernetes.io/master: ""
    ---
    apiVersion: configplane.juniper.net/v1alpha1
    kind: Controller
    metadata:
      name: contrail-k8s-controller
      namespace: contrail
    spec:
      common:
        replicas: REPLICAS
        containers:
        - image: REGISTRY/contrail-k8s-controller:TAG
          name: contrail-k8s-controller
        nodeSelector:
          node-role.kubernetes.io/master: ""
    ---
    apiVersion: configplane.juniper.net/v1alpha1
    kind: Kubemanager
    metadata:
      name: contrail-k8s-kubemanager
      namespace: contrail
    spec:
      autonomousSystem: ASN
      common:
        replicas: REPLICAS
        containers:
        - image: REGISTRY/contrail-k8s-kubemanager:TAG
          name: contrail-k8s-kubemanager
        nodeSelector:
          node-role.kubernetes.io/master: ""
    ---
    apiVersion: controlplane.juniper.net/v1alpha1
    kind: Control
    metadata:
      name: contrail-control
      namespace: contrail
    spec:
      autonomousSystem: ASN
      common:
        replicas: REPLICAS
        containers:
        - image: REGISTRY/contrail-control:TAG
          name: contrail-control
        - image: REGISTRY/contrail-telemetry-exporter:TAG
          name: contrail-control-telemetry-exporter
        initContainers:
        - image: REGISTRY/contrail-init:TAG
          name: contrail-init
        nodeSelector:
          node-role.kubernetes.io/master: ""
    ---
    apiVersion: dataplane.juniper.net/v1alpha1
    kind: Vrouter
    metadata:
      name: contrail-vrouter-masters
      namespace: contrail
    spec:
      agent:
        virtualHostInterface:
          gateway: GATEWAY
      common:
        containers:
        - image: REGISTRY/contrail-vrouter-agent:TAG
          name: contrail-vrouter-agent
        - image: REGISTRY/contrail-init:TAG
          name: contrail-watcher
        - image: REGISTRY/contrail-telemetry-exporter:TAG
          name: contrail-vrouter-telemetry-exporter
        initContainers:
        - image: REGISTRY/contrail-init:TAG
          name: contrail-init
        - image: REGISTRY/contrail-cni-init:TAG
          name: contrail-cni-init
        nodeSelector:
          node-role.kubernetes.io/master: ""
    ---
    apiVersion: dataplane.juniper.net/v1alpha1
    kind: Vrouter
    metadata:
      name: contrail-vrouter-nodes
      namespace: contrail
    spec:
      agent:
        virtualHostInterface:
          gateway: GATEWAY
      common:
        affinity:
          nodeAffinity:
            requiredDuringSchedulingIgnoredDuringExecution:
              nodeSelectorTerms:
              - matchExpressions:
                - key: node-role.kubernetes.io/master
                  operator: NotIn
                  values:
                  - ""
        containers:
        - image: REGISTRY/contrail-vrouter-agent:TAG
          name: contrail-vrouter-agent
        - image: REGISTRY/contrail-init:TAG
          name: contrail-watcher
        - image: REGISTRY/contrail-telemetry-exporter:TAG
          name: contrail-vrouter-telemetry-exporter
        initContainers:
        - image: REGISTRY/contrail-init:TAG
          name: contrail-init
        - image: REGISTRY/contrail-cni-init:TAG
          name: contrail-cni-init
kind: ConfigMap
metadata:
  creationTimestamp: null
  name: contrail-cr
  namespace: contrail
---
apiVersion: batch/v1
kind: Job
metadata:
  name: apply-contrail
  namespace: contrail
spec:
  backoffLimit: 4
  template:
    spec:
      containers:
      - command:
        - sh
        - -c
        - until kubectl wait --for condition=established --timeout=60s crd/apiservers.configplane.juniper.net; do echo 'waiting for apiserver crd'; sleep 2; done && until ls /tmp/contrail/contrail-cr.yaml; do sleep 2; echo 'waiting for manifest'; done && kubectl apply -f /tmp/contrail/contrail-cr.yaml && kubectl -n contrail delete job apply-contrail
        image: REGISTRY/contrail-k8s-applier:TAG
        name: applier
        volumeMounts:
        - mountPath: /tmp/contrail
          name: cr-volume
      hostNetwork: true
      nodeSelector:
        node-role.kubernetes.io/master: ""
      restartPolicy: Never
      serviceAccountName: contrail-serviceaccount
      tolerations:
      - effect: NoSchedule
        operator: Exists
      - effect: NoExecute
        operator: Exists
      volumes:
      - configMap:
          name: contrail-cr
        name: cr-volume`

func NewDeployer(scale int, gateway, podv4subnet, podv6subnet, servicev4subnet, servicev6subnet, registry, tag string, asn int) string {
	if tag == "" {
		tag = "latest"
	}
	template := strings.Replace(deployerTemplate, "GATEWAY", gateway, -1)
	template = strings.Replace(template, "PODV4SUBNET", podv4subnet, -1)
	template = strings.Replace(template, "PODV6SUBNET", podv6subnet, -1)
	template = strings.Replace(template, "SERVICEV4SUBNET", servicev4subnet, -1)
	template = strings.Replace(template, "SERVICEV6SUBNET", servicev6subnet, -1)
	template = strings.Replace(template, "REGISTRY", registry, -1)
	template = strings.Replace(template, "TAG", tag, -1)
	template = strings.Replace(template, "ASN", strconv.Itoa(asn), -1)
	return strings.Replace(template, "REPLICAS", strconv.Itoa(scale), -1)
}
