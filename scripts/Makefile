SHELL=/bin/bash

MINIKUBE_RUNNING := $(shell echo `minikube status | grep 'apiserver:' | awk '{print $$2}'`)

.PHONY: check-minikube deploy-to-minikube test clean

check-minikube:
	@[ $(MINIKUBE_RUNNING) != 'Running' ] && (echo 'minikube is not running' && exit 1) || echo 'minikube is running'

build-init-container-image: check-minikube
	(cd .. && docker build -t namespace-node-affinity-init-container -f build/DockerfileInitContainer .)
	(minikube cache add namespace-node-affinity-init-container)

build-webhook-image: check-minikube
	(cd .. && docker build -t namespace-node-affinity -f build/Dockerfile .)
	(minikube cache add namespace-node-affinity)

clean:
	(cd .. && kubectl delete -k deployments/overlays/local)
	(minikube cache delete namespace-node-affinity-init-container)
	(minikube cache delete namespace-node-affinity)

test:
	(cd .. && go test ./...)

deploy-to-minikube: build-init-container-image build-webhook-image
	(cd .. && kubectl apply -k deployments/overlays/local)
