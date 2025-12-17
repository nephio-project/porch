---
title: "Getting Started"
type: docs
weight: 3
description: "A set of guides for installing Porch prerequisites, the porchctl CLI, and deploying Porch components on a Kubernetes cluster." 
---

## Prerequisites

1. A supported OS (Linux/MacOS)
2. [git](https://git-scm.com/) ({{< params "version_git" >}})
3. [Docker](https://www.docker.com/get-started/) - either Docker Desktop or Docker Engine ({{< params "version_docker" >}})
4. [kubectl](https://kubernetes.io/docs/reference/kubectl/) - make sure that [kubectl context](https://kubernetes.io/docs/concepts/configuration/organize-cluster-access-kubeconfig/) configured with your cluster ({{< params "version_kube" >}})
5. [kpt](https://kpt.dev/installation/kpt-cli/) ({{< params "version_kpt" >}})
6. [The go programming language](https://go.dev/) ({{< params "version_go" >}})
7. A Kubernetes Cluster

{{% alert color="primary" title="Note:" %}}
The versions above relate to the latest tested versions confirmed to work and are **NOT** the only compatible versions.
{{% /alert %}}
