# Demo CSI Plugin for Kubernetes

A minimal, educational **Container Storage Interface (CSI)** driver written in Go.
It implements a *hostpath* driver — volumes are simply directories on the node
filesystem — so the code stays small enough to read in one sitting while still
exercising the full CSI RPC lifecycle.

---

## What Is CSI?

CSI is a **gRPC-based standard API** that decouples Kubernetes from any
particular storage backend. A CSI driver exposes three gRPC services:

| Service | Who calls it | Responsibility |
|---------|-------------|----------------|
| **Identity** | kubelet + sidecars | Driver name, version, capabilities, health |
| **Controller** | `external-provisioner` sidecar | Create / delete volumes (volume lifecycle) |
| **Node** | kubelet | Mount / unmount volumes inside pods |

The controller and node components can run in the same binary (as here) or as
separate binaries. In production they are usually separated.

---

## Architecture

```
┌──────────────────────────────────────────────────────────┐
│  kube-system namespace                                    │
│                                                          │
│  StatefulSet: demo-csi-controller (1 replica)            │
│  ┌────────────────────┐  ┌────────────────────────────┐  │
│  │  demo-csi-plugin   │  │  external-provisioner      │  │
│  │  (Controller+      │◄─┤  (watches PVCs, calls      │  │
│  │   Identity gRPC)   │  │   CreateVolume/Delete)     │  │
│  └────────────────────┘  └────────────────────────────┘  │
│                                                          │
│  DaemonSet: demo-csi-node (every node)                   │
│  ┌────────────────────┐  ┌────────────────────────────┐  │
│  │  demo-csi-plugin   │  │  node-driver-registrar     │  │
│  │  (Node + Identity  │◄─┤  (registers socket with    │  │
│  │   gRPC)            │  │   kubelet plugin API)      │  │
│  └────────────────────┘  └────────────────────────────┘  │
└──────────────────────────────────────────────────────────┘

Volume on host: /var/lib/demo-csi/volumes/<volumeID>/
Pod mount:      bind-mounted into pod at the declared mountPath
```

### Volume Lifecycle (step by step)

1. User creates a **PVC** referencing the `demo-csi` StorageClass.
2. `external-provisioner` calls **`CreateVolume`** → driver runs
   `os.MkdirAll("/var/lib/demo-csi/volumes/<name>")`.
3. Kubernetes creates a **PV** bound to the PVC.
4. A **Pod** that references the PVC is scheduled to a node.
5. kubelet calls **`NodePublishVolume`** → driver bind-mounts the volume
   directory into the pod's filesystem.
6. Pod runs; data lands in the host directory.
7. Pod is deleted → kubelet calls **`NodeUnpublishVolume`** → bind mount removed.
8. PVC is deleted → `external-provisioner` calls **`DeleteVolume`** → directory removed.

---

## Prerequisites

- Go 1.21+ (for local builds)
- Docker (for building the image)
- A single-node Kubernetes cluster: **minikube**, **kind**, or **k3s**
  (multi-node works too, but volumes live on one node's filesystem)

---

## Quick Start (minikube)

```bash
# 1. Start a cluster
minikube start

# 2. Build the image directly into minikube's Docker daemon
eval $(minikube docker-env)
make image

# 3. Deploy the driver
make deploy

# 4. Wait for pods to be ready
kubectl -n kube-system rollout status statefulset/demo-csi-controller
kubectl -n kube-system rollout status daemonset/demo-csi-node

# 5. Create a test PVC + Pod
make test-pod

# 6. Wait for the pod
kubectl wait --for=condition=Ready pod/demo-csi-test-pod --timeout=60s

# 7. Verify it worked
kubectl exec demo-csi-test-pod -- cat /data/hello.txt
```

Expected output:
```
Hello from demo CSI driver! <date>
```

---

## File Structure

```
csi-plugin/
├── cmd/
│   └── main.go               # Entry point: flags, starts the driver
├── pkg/driver/
│   ├── driver.go             # gRPC server setup + logging interceptor
│   ├── identity.go           # Identity service (GetPluginInfo, Probe, …)
│   ├── controller.go         # Controller service (CreateVolume, DeleteVolume, …)
│   └── node.go               # Node service (NodePublishVolume, …)
├── deploy/
│   ├── 01-rbac.yaml          # ServiceAccount + ClusterRole/Binding
│   ├── 02-csidriver.yaml     # CSIDriver object
│   ├── 03-storageclass.yaml  # StorageClass
│   ├── 04-controller.yaml    # StatefulSet (controller plugin + external-provisioner)
│   ├── 05-node.yaml          # DaemonSet (node plugin + node-driver-registrar)
│   └── 06-test-pod.yaml      # PVC + Pod for end-to-end verification
├── Dockerfile
├── Makefile
└── README.md
```

---

## Command-line Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--endpoint` | `unix:///var/lib/kubelet/plugins/demo.csi.example.com/csi.sock` | CSI gRPC endpoint |
| `--node-id` | hostname | Node identifier reported to Kubernetes |
| `--state-dir` | `/var/lib/demo-csi/volumes` | Root directory for volume subdirectories |

---

## Makefile Targets

| Target | Description |
|--------|-------------|
| `make build` | Compile binary to `bin/demo-csi-plugin` |
| `make image` | Build Docker image (`demo-csi-plugin:latest`) |
| `make push REGISTRY=…` | Tag and push to a registry |
| `make deploy` | `kubectl apply` all manifests in `deploy/` |
| `make undeploy` | Remove all deployed resources |
| `make test-pod` | Deploy the test PVC + Pod |
| `make clean-test` | Remove the test PVC + Pod |
| `make clean` | Remove build artifacts |

---

## Key CSI Concepts Illustrated

### gRPC Services
The driver implements all three required services in a single binary. In
production you would typically split the Controller service (StatefulSet) and
Node service (DaemonSet) into separate binaries, but the gRPC interfaces are
identical.

### Idempotency
CSI requires all RPCs to be idempotent. Notice:
- `CreateVolume` uses `os.MkdirAll` — creating an already-existing dir is a no-op.
- `DeleteVolume` uses `os.RemoveAll` — deleting a non-existent path is a no-op.
- `NodeUnpublishVolume` ignores `EINVAL` (path not mounted).

### Sidecars
Kubernetes provides official sidecar containers that translate Kubernetes events
into CSI RPC calls so your driver doesn't need Kubernetes API client code:
- **`external-provisioner`** → calls `CreateVolume` / `DeleteVolume`
- **`node-driver-registrar`** → registers the driver socket with kubelet

### Bind Mounts
`NodePublishVolume` uses a Linux bind mount (`MS_BIND`) to make the volume
directory appear inside the pod's mount namespace. No special filesystem is
involved — it's just a directory.

---

## Limitations (by design — this is a demo)

- **No capacity enforcement** — volumes share the node's root filesystem.
- **Single-node affinity** — volumes live on whichever node the controller ran
  on; pods must schedule to the same node (guaranteed on single-node clusters).
- **No snapshots, cloning, expansion, or metrics**.
- **No `ControllerPublishVolume`** — `attachRequired: false` in the CSIDriver
  spec tells Kubernetes to skip the attach step.

---

## License

MIT
