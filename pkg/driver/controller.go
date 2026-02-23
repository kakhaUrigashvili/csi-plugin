package driver

import (
	"context"
	"os"
	"path/filepath"

	csi "github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/klog/v2"
)

type controllerServer struct {
	d *Driver
	// Embed the unimplemented server so that we satisfy the interface for RPC
	// methods we don't implement (e.g. CreateSnapshot, ListVolumes, …).
	csi.UnimplementedControllerServer
}

// CreateVolume creates a directory on the host to back the requested volume.
// Using the volume name as the ID makes the operation idempotent.
func (s *controllerServer) CreateVolume(_ context.Context, req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {
	if req.GetName() == "" {
		return nil, status.Error(codes.InvalidArgument, "volume name is required")
	}
	if len(req.GetVolumeCapabilities()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "volume capabilities are required")
	}

	// Use the name as the volume ID so repeated calls with the same name are
	// idempotent (re-create returns the same volume).
	volumeID := req.GetName()
	volumeDir := filepath.Join(s.d.stateDir, volumeID)

	if err := os.MkdirAll(volumeDir, 0750); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to create volume dir %q: %v", volumeDir, err)
	}

	klog.Infof("CreateVolume: id=%s path=%s", volumeID, volumeDir)

	// Determine capacity — we track it for the response but don't enforce it
	// (hostpath volumes share the underlying filesystem).
	capacityBytes := int64(0)
	if cr := req.GetCapacityRange(); cr != nil {
		capacityBytes = cr.GetRequiredBytes()
	}

	return &csi.CreateVolumeResponse{
		Volume: &csi.Volume{
			VolumeId:      volumeID,
			CapacityBytes: capacityBytes,
			VolumeContext: req.GetParameters(),
		},
	}, nil
}

// DeleteVolume removes the directory that backs the volume.
// It is idempotent: deleting a non-existent volume succeeds.
func (s *controllerServer) DeleteVolume(_ context.Context, req *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {
	if req.GetVolumeId() == "" {
		return nil, status.Error(codes.InvalidArgument, "volume ID is required")
	}

	volumeDir := filepath.Join(s.d.stateDir, req.GetVolumeId())
	if err := os.RemoveAll(volumeDir); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to delete volume dir %q: %v", volumeDir, err)
	}

	klog.Infof("DeleteVolume: id=%s path=%s", req.GetVolumeId(), volumeDir)
	return &csi.DeleteVolumeResponse{}, nil
}

// ValidateVolumeCapabilities confirms that the requested access modes are
// supported. We support ReadWriteOnce and ReadOnlyMany.
func (s *controllerServer) ValidateVolumeCapabilities(_ context.Context, req *csi.ValidateVolumeCapabilitiesRequest) (*csi.ValidateVolumeCapabilitiesResponse, error) {
	if req.GetVolumeId() == "" {
		return nil, status.Error(codes.InvalidArgument, "volume ID is required")
	}
	if len(req.GetVolumeCapabilities()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "volume capabilities are required")
	}

	for _, cap := range req.GetVolumeCapabilities() {
		mode := cap.GetAccessMode().GetMode()
		if mode != csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER &&
			mode != csi.VolumeCapability_AccessMode_SINGLE_NODE_READER_ONLY &&
			mode != csi.VolumeCapability_AccessMode_MULTI_NODE_READER_ONLY {
			return &csi.ValidateVolumeCapabilitiesResponse{
				Message: "unsupported access mode",
			}, nil
		}
	}

	return &csi.ValidateVolumeCapabilitiesResponse{
		Confirmed: &csi.ValidateVolumeCapabilitiesResponse_Confirmed{
			VolumeCapabilities: req.GetVolumeCapabilities(),
		},
	}, nil
}

// ControllerGetCapabilities reports the capabilities this controller implements.
func (s *controllerServer) ControllerGetCapabilities(_ context.Context, _ *csi.ControllerGetCapabilitiesRequest) (*csi.ControllerGetCapabilitiesResponse, error) {
	return &csi.ControllerGetCapabilitiesResponse{
		Capabilities: []*csi.ControllerServiceCapability{
			{
				Type: &csi.ControllerServiceCapability_Rpc{
					Rpc: &csi.ControllerServiceCapability_RPC{
						Type: csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME,
					},
				},
			},
		},
	}, nil
}
