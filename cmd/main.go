package main

import (
	"flag"
	"os"

	"github.com/example/demo-csi-plugin/pkg/driver"
	"k8s.io/klog/v2"
)

var (
	endpoint = flag.String("endpoint", "unix:///var/lib/kubelet/plugins/demo.csi.example.com/csi.sock",
		"CSI endpoint (unix:// or tcp://)")
	nodeID = flag.String("node-id", "",
		"Node ID (defaults to hostname)")
	stateDir = flag.String("state-dir", "/var/lib/demo-csi/volumes",
		"Directory where volume subdirectories are created")
)

func main() {
	klog.InitFlags(nil)
	flag.Parse()

	if *nodeID == "" {
		hostname, err := os.Hostname()
		if err != nil {
			klog.Fatalf("Failed to get hostname: %v", err)
		}
		*nodeID = hostname
	}

	klog.Infof("Starting demo CSI plugin: node=%s endpoint=%s stateDir=%s",
		*nodeID, *endpoint, *stateDir)

	d, err := driver.New(*nodeID, *stateDir)
	if err != nil {
		klog.Fatalf("Failed to create driver: %v", err)
	}

	if err := d.Run(*endpoint); err != nil {
		klog.Fatalf("Driver exited with error: %v", err)
	}
}
