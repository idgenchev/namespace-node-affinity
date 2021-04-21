SHELL=/bin/bash

MINIKUBE_RUNNING := $(shell echo `minikube status | grep 'apiserver:' | awk '{print $$2}'`)

.PHONY: minikube-env
minikube-env:
	@[ $(MINIKUBE_RUNNING) == 'Running' ] && eval $$(minikube docker-env) || (echo 'minikube is not running' && exit 1)

build-init-container-image:
	(cd .. && docker build -t namespace-node-affinity-init-container -f build/DockerfileInitContainer .)

build-webhook-image:
	(cd .. && docker build -t namespace-node-affinity -f build/Dockerfile .)

clean:
	(cd .. && kubectl delete -k deployments/overlays/local)

test:
	(cd .. && go test ./...)

deploy-to-minikube: minikube-env build-init-container-image build-webhook-image
	(cd .. && kubectl apply -k deployments/overlays/local)
