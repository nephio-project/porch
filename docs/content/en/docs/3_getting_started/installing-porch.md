---
title: "Installing Porch"
type: docs
weight: 2
description: Install guide for the porch system on a Kubernetes cluster.
---

## Deploying Porch on a cluster

Create a new directory for the kpt package and path inside of it

```bash
mkdir porch-{{% params "latestTag" %}} && cd porch-{{% params "latestTag" %}}
```

Download the latest Porch kpt package blueprint:

```bash
curl -LO "https://github.com/nephio-project/porch/releases/download/v{{% params "latestTag" %}}/porch_blueprint.tar.gz"
```

Extract the Porch kpt package contents:

```bash
tar -xzf porch_blueprint.tar.gz
```

Initialize and apply the Porch kpt package:

```bash
kpt live init && kpt live apply
```

You can check that Porch is up and running with the following command:

```bash
kubectl get all -n porch-system
```

A healthy Porch install should look like the following:

```bash
NAME                                   READY   STATUS    RESTARTS   AGE
pod/function-runner-567ddc76d-7k8sj    1/1     Running   0          4m3s
pod/function-runner-567ddc76d-x75lv    1/1     Running   0          4m3s
pod/porch-controllers-d8dfccb4-8lc6j   1/1     Running   0          4m3s
pod/porch-server-7dc5d7cd4f-smhf5      1/1     Running   0          4m3s

NAME                      TYPE        CLUSTER-IP      EXTERNAL-IP   PORT(S)            AGE
service/api               ClusterIP   10.96.108.221   <none>        443/TCP,8443/TCP   4m3s
service/function-runner   ClusterIP   10.96.237.108   <none>        9445/TCP           4m3s

NAME                                READY   UP-TO-DATE   AVAILABLE   AGE
deployment.apps/function-runner     2/2     2            2           4m3s
deployment.apps/porch-controllers   1/1     1            1           4m3s
deployment.apps/porch-server        1/1     1            1           4m3s

NAME                                         DESIRED   CURRENT   READY   AGE
replicaset.apps/function-runner-567ddc76d    2         2         2       4m3s
replicaset.apps/porch-controllers-d8dfccb4   1         1         1       4m3s
replicaset.apps/porch-server-7dc5d7cd4f      1         1         1       4m3s
```
