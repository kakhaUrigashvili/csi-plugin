// Package driver implements a simple hostpath CSI driver for educational purposes.
// It demonstrates the three gRPC services required by the CSI spec:
// Identity, Controller, and Node.
package driver

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"

	csi "github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/klog/v2"
)

const driverName = "demo.csi.example.com"

// Driver holds the state for our CSI plugin.
type Driver struct {
	nodeID   string
	stateDir string
}

// New creates a new Driver instance.
func New(nodeID, stateDir string) (*Driver, error) {
	if err := os.MkdirAll(stateDir, 0750); err != nil {
		return nil, fmt.Errorf("failed to create state dir %q: %w", stateDir, err)
	}
	return &Driver{nodeID: nodeID, stateDir: stateDir}, nil
}

// Run parses the endpoint, starts the gRPC server, and blocks until it stops.
func (d *Driver) Run(endpoint string) error {
	u, err := url.Parse(endpoint)
	if err != nil {
		return fmt.Errorf("invalid endpoint %q: %w", endpoint, err)
	}

	var addr string
	switch u.Scheme {
	case "unix":
		addr = filepath.Join(u.Host, u.Path)
		// Remove a stale socket left over from a previous crash.
		if err := os.Remove(addr); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to remove stale socket %q: %w", addr, err)
		}
		if err := os.MkdirAll(filepath.Dir(addr), 0750); err != nil {
			return fmt.Errorf("failed to create socket dir: %w", err)
		}
	case "tcp":
		addr = u.Host
	default:
		return fmt.Errorf("unsupported endpoint scheme %q (use unix:// or tcp://)", u.Scheme)
	}

	listener, err := net.Listen(u.Scheme, addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s://%s: %w", u.Scheme, addr, err)
	}

	server := grpc.NewServer(grpc.UnaryInterceptor(logInterceptor))

	csi.RegisterIdentityServer(server, &identityServer{d: d})
	csi.RegisterControllerServer(server, &controllerServer{d: d})
	csi.RegisterNodeServer(server, &nodeServer{d: d})

	klog.Infof("CSI driver listening on %s://%s", u.Scheme, addr)
	return server.Serve(listener)
}

// logInterceptor logs every incoming RPC together with any error that is returned.
func logInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	klog.V(4).Infof("RPC → %s", info.FullMethod)
	resp, err := handler(ctx, req)
	if err != nil {
		st, _ := status.FromError(err)
		if st.Code() != codes.OK {
			klog.Errorf("RPC %s failed: %v", info.FullMethod, err)
		}
	}
	return resp, err
}
