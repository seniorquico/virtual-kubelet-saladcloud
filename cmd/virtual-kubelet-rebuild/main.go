package main

import (
	"context"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"github.com/mitchellh/go-homedir"
	dto "github.com/prometheus/client_model/go"
	"github.com/virtual-kubelet/virtual-kubelet/errdefs"
	"github.com/virtual-kubelet/virtual-kubelet/node"
	"github.com/virtual-kubelet/virtual-kubelet/node/api"
	"github.com/virtual-kubelet/virtual-kubelet/node/api/statsv1alpha1"
	"github.com/virtual-kubelet/virtual-kubelet/node/nodeutil"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var k8sVersion = "v1.29.8"

func main() {
	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt)
	defer func() {
		signal.Stop(signalChan)
		cancel()
	}()
	go func() {
		select {
		case <-signalChan:
			cancel()
		case <-ctx.Done():
		}
		<-signalChan
		os.Exit(2)
	}()

	home, err := homedir.Dir()
	k8sConfig := os.Getenv("KUBECONFIG")
	if err == nil && home != "" {
		k8sConfig = filepath.Join(home, ".kube", "config")
	}
	k8sClient, err := nodeutil.ClientsetFromEnv(k8sConfig)
	if err != nil {
		panic(err)
	}
	withClient := func(cfg *nodeutil.NodeConfig) error {
		return nodeutil.WithClient(k8sClient)(cfg)
	}

	withNodeConfiguration := func(cfg *nodeutil.NodeConfig) error {
		// cfg.NodeSpec.ObjectMeta.Labels["kubernetes.io/arch"] = "amd64"
		// cfg.NodeSpec.ObjectMeta.Labels["kubernetes.io/os"] = "linux"
		// cfg.NodeSpec.ObjectMeta.Labels["beta.kubernetes.io/arch"] = "amd64"
		// cfg.NodeSpec.ObjectMeta.Labels["beta.kubernetes.io/os"] = "linux"
		cfg.NodeSpec.ObjectMeta.Labels["alpha.service-controller.kubernetes.io/exclude-balancer"] = "true"
		cfg.NodeSpec.ObjectMeta.Labels["node.kubernetes.io/exclude-from-external-load-balancers"] = "true"
		cfg.NodeSpec.Status.Allocatable = nodeCapacity()
		cfg.NodeSpec.Status.Capacity = nodeCapacity()
		cfg.NodeSpec.Status.Conditions = nodeConditions()
		cfg.NodeSpec.Status.NodeInfo.Architecture = "amd64"
		cfg.NodeSpec.Status.NodeInfo.OperatingSystem = "linux"
		cfg.NodeSpec.Status.NodeInfo.KubeletVersion = strings.Join([]string{k8sVersion, "vk-saladcloud", "dev"}, "-")
		return nil
	}

	withTaint := func(cfg *nodeutil.NodeConfig) error {
		cfg.NodeSpec.Spec.Taints = append(cfg.NodeSpec.Spec.Taints, v1.Taint{
			Key:    "virtual-kubelet.io/provider",
			Value:  "saladcloud",
			Effect: corev1.TaintEffectNoSchedule,
		})
		return nil
	}

	node, err := nodeutil.NewNode(
		"vk-saladcloud",
		func(pc nodeutil.ProviderConfig) (nodeutil.Provider, node.NodeProvider, error) {
			return &SaladCloudProvider{}, nil, nil
		},
		withClient,
		withNodeConfiguration,
		withTaint)
	if err != nil {
		panic(err)
	}

	go func() {
		err = node.Run(ctx)
		if err != nil {
			panic(err)
		}
	}()

	err = node.WaitReady(ctx, 30*time.Second)
	if err != nil {
		panic(err)
	}

	<-node.Done()
	err = node.Err()
	if err != nil {
		panic(err)
	}
}

func nodeCapacity() corev1.ResourceList {
	resourceList := corev1.ResourceList{
		v1.ResourceCPU:    resource.MustParse("10000"),
		v1.ResourceMemory: resource.MustParse("4Ti"),
		v1.ResourcePods:   resource.MustParse("5000"),
		"nvidia.com/gpu":  resource.MustParse("5000"),
	}
	return resourceList
}

func nodeConditions() []corev1.NodeCondition {
	currentTime := metav1.Now()
	return []corev1.NodeCondition{
		{
			Type:               corev1.NodeReady,
			Status:             corev1.ConditionTrue,
			Reason:             "KubeletReady",
			Message:            "kubelet is posting ready status",
			LastHeartbeatTime:  currentTime,
			LastTransitionTime: currentTime,
		},
		{
			Type:               corev1.NodeDiskPressure,
			Status:             corev1.ConditionFalse,
			Reason:             "KubeletHasNoDiskPressure",
			Message:            "kubelet has no disk pressure",
			LastHeartbeatTime:  currentTime,
			LastTransitionTime: currentTime,
		},
		{
			Type:               corev1.NodeMemoryPressure,
			Status:             corev1.ConditionFalse,
			Reason:             "KubeletHasSufficientMemory",
			Message:            "kubelet has sufficient memory available",
			LastHeartbeatTime:  currentTime,
			LastTransitionTime: currentTime,
		},
		{
			Type:               corev1.NodeNetworkUnavailable,
			Status:             corev1.ConditionFalse,
			Reason:             "SaladCloudContainerGatewayReady",
			Message:            "SaladCloud container gateway is posting ready status",
			LastHeartbeatTime:  currentTime,
			LastTransitionTime: currentTime,
		},
		{
			Type:               corev1.NodePIDPressure,
			Status:             corev1.ConditionFalse,
			Reason:             "KubeletHasSufficientPID",
			Message:            "kubelet has sufficient PID available",
			LastHeartbeatTime:  currentTime,
			LastTransitionTime: currentTime,
		},
	}
}

// nodeutil.Provider, node.PodLifecycleHandler, PodNotifier
type SaladCloudProvider struct {
}

// CreatePod takes a Kubernetes Pod and deploys it within the provider.
func (p *SaladCloudProvider) CreatePod(ctx context.Context, pod *corev1.Pod) error {
	panic("not implemented") // TODO: Implement
}

// UpdatePod takes a Kubernetes Pod and updates it within the provider.
func (p *SaladCloudProvider) UpdatePod(ctx context.Context, pod *corev1.Pod) error {
	panic("not implemented") // TODO: Implement
}

// DeletePod takes a Kubernetes Pod and deletes it from the provider. Once a pod is deleted, the provider is
// expected to call the NotifyPods callback with a terminal pod status where all the containers are in a terminal
// state, as well as the pod. DeletePod may be called multiple times for the same pod.
func (p *SaladCloudProvider) DeletePod(ctx context.Context, pod *corev1.Pod) error {
	panic("not implemented") // TODO: Implement
}

// GetPod retrieves a pod by name from the provider (can be cached).
// The Pod returned is expected to be immutable, and may be accessed
// concurrently outside of the calling goroutine. Therefore it is recommended
// to return a version after DeepCopy.
func (p *SaladCloudProvider) GetPod(ctx context.Context, namespace string, name string) (*corev1.Pod, error) {
	// panic("not implemented") // TODO: Implement
	return nil, errdefs.NotFound("pod not found")
}

// GetPodStatus retrieves the status of a pod by name from the provider.
// The PodStatus returned is expected to be immutable, and may be accessed
// concurrently outside of the calling goroutine. Therefore it is recommended
// to return a version after DeepCopy.
func (p *SaladCloudProvider) GetPodStatus(ctx context.Context, namespace string, name string) (*corev1.PodStatus, error) {
	panic("not implemented") // TODO: Implement
}

// GetPods retrieves a list of all pods running on the provider (can be cached).
// The Pods returned are expected to be immutable, and may be accessed
// concurrently outside of the calling goroutine. Therefore it is recommended
// to return a version after DeepCopy.
func (p *SaladCloudProvider) GetPods(_ context.Context) ([]*corev1.Pod, error) {
	return make([]*corev1.Pod, 0), nil // TODO: Implement
}

// NotifyPods instructs the notifier to call the passed in function when
// the pod status changes. It should be called when a pod's status changes.
//
// The provided pointer to a Pod is guaranteed to be used in a read-only
// fashion. The provided pod's PodStatus should be up to date when
// this function is called.
//
// NotifyPods must not block the caller since it is only used to register the callback.
// The callback passed into `NotifyPods` may block when called.
func (p *SaladCloudProvider) NotifyPods(context.Context, func(*corev1.Pod)) {
	// TODO: Implement
}

// GetContainerLogs retrieves the logs of a container by name from the provider.
func (p *SaladCloudProvider) GetContainerLogs(ctx context.Context, namespace string, podName string, containerName string, opts api.ContainerLogOpts) (io.ReadCloser, error) {
	panic("not implemented") // TODO: Implement
}

// RunInContainer executes a command in a container in the pod, copying data
// between in/out/err and the container's stdin/stdout/stderr.
func (p *SaladCloudProvider) RunInContainer(ctx context.Context, namespace string, podName string, containerName string, cmd []string, attach api.AttachIO) error {
	panic("not implemented") // TODO: Implement
}

// AttachToContainer attaches to the executing process of a container in the pod, copying data
// between in/out/err and the container's stdin/stdout/stderr.
func (p *SaladCloudProvider) AttachToContainer(ctx context.Context, namespace string, podName string, containerName string, attach api.AttachIO) error {
	panic("not implemented") // TODO: Implement
}

// GetStatsSummary gets the stats for the node, including running pods
func (p *SaladCloudProvider) GetStatsSummary(_ context.Context) (*statsv1alpha1.Summary, error) {
	panic("not implemented") // TODO: Implement
}

// GetMetricsResource gets the metrics for the node, including running pods
func (p *SaladCloudProvider) GetMetricsResource(_ context.Context) ([]*dto.MetricFamily, error) {
	panic("not implemented") // TODO: Implement
}

// PortForward forwards a local port to a port on the pod
func (p *SaladCloudProvider) PortForward(ctx context.Context, namespace string, pod string, port int32, stream io.ReadWriteCloser) error {
	panic("not implemented") // TODO: Implement
}
