IMAGE      ?= demo-csi-plugin
TAG        ?= latest
REGISTRY   ?= # set to e.g. docker.io/youruser to push to a registry

.PHONY: build push deploy undeploy test-pod clean

## build: compile the binary locally (requires Go 1.21+)
build:
	CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/demo-csi-plugin ./cmd/

## image: build the container image
image:
	docker build -t $(IMAGE):$(TAG) .

## push: build and push the image to $(REGISTRY)
push: image
	@if [ -z "$(REGISTRY)" ]; then echo "Set REGISTRY before pushing, e.g. make push REGISTRY=docker.io/youruser"; exit 1; fi
	docker tag $(IMAGE):$(TAG) $(REGISTRY)/$(IMAGE):$(TAG)
	docker push $(REGISTRY)/$(IMAGE):$(TAG)

## deploy: apply all Kubernetes manifests
deploy:
	kubectl apply -f deploy/01-rbac.yaml
	kubectl apply -f deploy/02-csidriver.yaml
	kubectl apply -f deploy/03-storageclass.yaml
	kubectl apply -f deploy/04-controller.yaml
	kubectl apply -f deploy/05-node.yaml

## undeploy: remove all Kubernetes resources created by deploy
undeploy:
	kubectl delete -f deploy/05-node.yaml        --ignore-not-found
	kubectl delete -f deploy/04-controller.yaml  --ignore-not-found
	kubectl delete -f deploy/03-storageclass.yaml --ignore-not-found
	kubectl delete -f deploy/02-csidriver.yaml    --ignore-not-found
	kubectl delete -f deploy/01-rbac.yaml         --ignore-not-found

## test-pod: apply the test PVC + Pod
test-pod:
	kubectl apply -f deploy/06-test-pod.yaml

## clean-test: remove the test PVC + Pod
clean-test:
	kubectl delete -f deploy/06-test-pod.yaml --ignore-not-found

## clean: remove build artifacts
clean:
	rm -rf bin/

## help: print this help
help:
	@grep -E '^## ' Makefile | sed 's/## //'
