package driver

import (
	"context"
	"os"
	"path/filepath"
	"syscall"

	csi "github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/klog/v2"
)

type nodeServer struct {
	d *Driver
	// Embed the unimplemented server to satisfy methods we don't implement.
	csi.UnimplementedNodeServer
}

// NodePublishVolume bind-mounts the volume directory into the pod.
//
// Kubernetes calls this after CreateVolume. The volume directory was created by
// the controller; we just need to make it visible inside the pod's namespace by
// bind-mounting it at the target path.
func (s *nodeServer) NodePublishVolume(_ context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	if req.GetVolumeId() == "" {
		return nil, status.Error(codes.InvalidArgument, "volume ID is required")
	}
	if req.GetTargetPath() == "" {
		return nil, status.Error(codes.InvalidArgument, "target path is required")
	}
	if req.GetVolumeCapability() == nil {
		return nil, status.Error(codes.InvalidArgument, "volume capability is required")
	}

	volumeDir := filepath.Join(s.d.stateDir, req.GetVolumeId())
	targetPath := req.GetTargetPath()

	// Ensure the source directory exists (it should have been created by
	// CreateVolume on the controller, but on single-node clusters that is us).
	if err := os.MkdirAll(volumeDir, 0750); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to create volume dir %q: %v", volumeDir, err)
	}

	// The target path is the directory inside the pod where the volume appears.
	if err := os.MkdirAll(targetPath, 0750); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to create target dir %q: %v", targetPath, err)
	}

	flags := uintptr(syscall.MS_BIND)
	if req.GetReadonly() {
		flags |= syscall.MS_RDONLY
	}

	if err := syscall.Mount(volumeDir, targetPath, "", flags, ""); err != nil {
		return nil, status.Errorf(codes.Internal, "bind mount %q → %q failed: %v", volumeDir, targetPath, err)
	}

	klog.Infof("NodePublishVolume: id=%s src=%s target=%s", req.GetVolumeId(), volumeDir, targetPath)
	return &csi.NodePublishVolumeResponse{}, nil
}

// NodeUnpublishVolume unmounts the bind mount created by NodePublishVolume.
// It is idempotent: if the path is not mounted (EINVAL) we treat it as success.
func (s *nodeServer) NodeUnpublishVolume(_ context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	if req.GetVolumeId() == "" {
		return nil, status.Error(codes.InvalidArgument, "volume ID is required")
	}
	if req.GetTargetPath() == "" {
		return nil, status.Error(codes.InvalidArgument, "target path is required")
	}

	targetPath := req.GetTargetPath()

	if err := syscall.Unmount(targetPath, 0); err != nil {
		// EINVAL means the path is not mounted — already unpublished, which is fine.
		if err == syscall.EINVAL {
			klog.V(4).Infof("NodeUnpublishVolume: %q is not mounted, skipping", targetPath)
			return &csi.NodeUnpublishVolumeResponse{}, nil
		}
		return nil, status.Errorf(codes.Internal, "unmount %q failed: %v", targetPath, err)
	}

	klog.Infof("NodeUnpublishVolume: id=%s target=%s", req.GetVolumeId(), targetPath)
	return &csi.NodeUnpublishVolumeResponse{}, nil
}

// NodeGetCapabilities reports which optional node-side capabilities we support.
// We keep this simple: no STAGE_UNSTAGE_VOLUME, no expansion, no stats.
func (s *nodeServer) NodeGetCapabilities(_ context.Context, _ *csi.NodeGetCapabilitiesRequest) (*csi.NodeGetCapabilitiesResponse, error) {
	return &csi.NodeGetCapabilitiesResponse{
		Capabilities: []*csi.NodeServiceCapability{},
	}, nil
}

// NodeGetInfo returns the node ID that the driver was started with.
// The external-provisioner uses this to set node affinity on PVs.
func (s *nodeServer) NodeGetInfo(_ context.Context, _ *csi.NodeGetInfoRequest) (*csi.NodeGetInfoResponse, error) {
	return &csi.NodeGetInfoResponse{
		NodeId: s.d.nodeID,
	}, nil
}
