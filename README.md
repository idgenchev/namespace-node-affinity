![CI](https://github.com/idgenchev/namespace-node-affinity/actions/workflows/ci.yaml/badge.svg?branch=main)
[![Go Report Card](https://goreportcard.com/badge/github.com/idgenchev/namespace-node-affinity)](https://goreportcard.com/report/github.com/idgenchev/namespace-node-affinity)
[![codecov](https://codecov.io/gh/idgenchev/test-golang-config/branch/main/graph/badge.svg?token=6Y4A13AFB5)](https://codecov.io/gh/idgenchev/test-golang-config)

# Namespace Node Affinity

Namespace Node Affinity is a Kubernetes mutating webhook which provides the ability to define node affinity and/or tolerations for pods on a namespace level.

It is a replacement for the [PodNodeSelector](https://kubernetes.io/docs/reference/access-authn-authz/admission-controllers/#podnodeselector) admission controller and it is useful when using a managed k8s control plane such as [GKE](https://cloud.google.com/kubernetes-engine) or [EKS](https://aws.amazon.com/eks) where you do not have the ability to enable additional admission controller plugins and the [PodNodeSelector](https://kubernetes.io/docs/reference/access-authn-authz/admission-controllers/#podnodeselector) might not be available. The only admission controller plugin required to run the namespace-node-affinity mutating webhook is the `MutatingAdmissionWebhook` which is already enabled on most managed Kubernetes services such as [EKS](https://docs.aws.amazon.com/eks/latest/userguide/platform-versions.html).

It might still be useful on [AKS](https://azure.microsoft.com/en-gb/services/kubernetes-service/) where the [PodNodeSelector](https://kubernetes.io/docs/reference/access-authn-authz/admission-controllers/#podnodeselector) admission controller is [readily available](https://docs.microsoft.com/en-us/azure/aks/faq#what-kubernetes-admission-controllers-does-aks-support-can-admission-controllers-be-added-or-removed) as using `namespace-node-affinity` allows a litte bit more flexibility than the node selector by allowing you to set node affinity (only `requiredDuringSchedulingIgnoredDuringExecution` is supported for now) for all pods in the namespace.

# Deployment

The easiest way to deploy the namespace-node-affinity mutating webhook is to apply the kustomizations in the `deployments` directory:
```
kubectl apply -k deployments/base
```

This will create the following:
 * namespace-node-affinity ServiceAccount
 * namespace-node-affinity Role
 * namespace-node-affinity RoleBinding
 * namespace-node-affinity ClusterRole
 * namespace-node-affinity ClusterRoleBinding
 * namespace-node-affinity Service
 * namespace-node-affinity Deployment

> Note that this will use the latest images on [Docker Hub](https://hub.docker.com/repository/docker/idgenchev/namespace-node-affinity). If you like to use a specific tag you can use the kustomizations in [deployments](/deployments/) as base and override the images in the Deployment with the desired tag.

The Deployment includes an init container which generates a CA and a certificate and key pair for the webhook server and will create/update the MutatingWebhookConfiguration with the generated CA bundle which will be loaded by the Kubernetes API server and used to verify the serving certificates of the namespace-node-affinity mutating webhook. Using this init container allows for a quick and easy deployment of the namespace-node-affinity webhook, but is not recommended for production. For production use it is recommended to use a tool such as [cert-manager](https://cert-manager.io) to manage the certificates for the namespace-node-affinity mutating webhook.

Docker images for the webhook are available for multiple platforms [here](https://hub.docker.com/repository/docker/idgenchev/namespace-node-affinity). Images for the init container are available [here](https://hub.docker.com/repository/docker/idgenchev/namespace-node-affinity-init-container).

# Required Permissions

The namespace-node-affinity webhook requires `get` permissions for `configmaps` in the namespace where the centralised config is deployed.

The init container (if used) requires `get`, `create` and `update` for `mutatingwebhookconfigurations` in the `admissionregistration.k8s.io` api group to create or update the MutatingWebhookConfiguration.

The `Role` and `ClusterRole` included in [deployments](/deployments/) already include all of the required permissions and the supplied `RoleBinding` and `ClusterRoleBinding` binds the `Role` and `ClusterRole` to the `ServiceAccount` used by the webhook.

# Configuration

To enable the namespace-node-affinity mutating webhook on a namespace you simply have to label the namespace with `namespace-node-affinity=enabled`.
```
kubectl label ns my-namespace namespace-node-affinity=enabled
```

Each namespace with the `namespace-node-affinity=enabled` label will also need an entry in the `ConfigMap` where the configuration for the webhook is stored. The config for each namespace can be in either JSON or YAML format and must have at least one of `nodeSelectorTerms` or `tolerations`. The `nodeSelectorTerms` from the config will be added as `requiredDuringSchedulingIgnoredDuringExecution` node affinity type to each pod that is created in the labeled namespace. An example configuration can be found in [examples/sample_configmap.yaml](/examples/sample_configmap.yaml).

More information on how node affinity works can be found [here](https://kubernetes.io/docs/concepts/scheduling-eviction/assign-pod-node/#node-affinity).
More information on how taints and tolerations work can be found [here](https://kubernetes.io/docs/concepts/scheduling-eviction/taint-and-toleration/).

# Failure Modes

When using the provided init container to create the mutating webhook configuration, the namespace-node-affinity mutating webhook will fail silently so pods can still be created on the cluster if the webhook has been misconfigured. The affected namespace can be seen in the `AdmissionReview.Namespace`.

 * Missing `namespace-node-affinity` `ConfigMap`
```
time="2021-04-10T09:35:06Z" level=info msg="Received AdmissionReview: {...}
time="2021-04-10T09:35:06Z" level=error msg="missing configuration: configmaps \"namespace-node-affinity\" not found"
```

 * Missing entry for the namespace in the `ConfigMap`
```
time="2021-09-03T17:32:16Z" level=info msg="Received AdmissionReview: {...}
time="2021-09-03T17:32:16Z" level=error msg="missing configuration: for testing-ns-e"
```

 * Both `nodeSelectorTerms` and `tolerations` are missing from the entry for the namespace in the `ConfigMap`
```
time="2021-09-03T17:38:46Z" level=info msg="Received AdmissionReview: {...}
time="2021-09-03T17:38:46Z" level=error msg="invalid configuration: at least one of nodeSelectorTerms or tolerations needs to be specified for testing-ns-d"
```

 * Invalid `nodeSelectorTerms` or `tolerations` in the `namespace-node-affinity` `ConfigMap`
```
time="2021-04-10T09:40:59Z" level=info msg="Received AdmissionReview: {...}
time="2021-04-10T09:40:59Z" level=error msg="invalid configuration: error unmarshaling JSON: while decoding JSON: json: cannot unmarshal string into Go struct field NamespaceConfig.nodeSelectorTerms of type []v1.NodeSelectorTerm"
```

# Contributing

Want to contribute? Awesome! The easiest way to show your support is to star the project, or to raise issues. If you want to open a pull request, please follow the [contributing guidelines](/.github/CONTRIBUTING.md).

Thanks for your support, it is much appreciated!

# License

Apache-2.0. See LICENSE for more details.
