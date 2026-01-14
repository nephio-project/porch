---
title: "Function runner pod templating"
type: docs
weight: 4
description: 
---

<div style="border: 1px solid red; background-color: #ffe6e6; color: #b30000; padding: 1em; margin-bottom: 1em;">
  <strong>⚠️ Outdated Notice:</strong> This page refers to an older version of the documentation. This content has simply been moved into its relevant new section here and must be checked, modified, rewritten, updated, or removed entirely.
</div>

## Overview

The `porch-fn-runner` implements a simple function-as-a-service for executing kpt functions, running
the necessary kpt functions wrapped in a GRPC server. The function of the `porch-fn-runner` is to
start up a number of function evaluator pods for each of the kpt functions, along with a front-end
service, pointing to its respective pod. As with any operator that manages pods, it is good to
provide some templating and parameterization capabilities of the pods that will be managed by the
function runner.

## Contract for writing pod templates

The following contract needs to be fulfilled by any function evaluator pod template:

1. There is a container. The container is named "function".
2. The entry point of the “function” container will start the wrapper GRPC server.
3. The image of the “function” container can be set to the image of the kpt function without
   impacting the starting of the entry point.
4. The arguments of the “function” container can be appended with the entries from the Dockerfile
   ENTRYPOINT of the kpt function image.

## Enabling pod templating on function runner

A ConfigMap with the pod template should be created in the namespace where the porch-fn-runner pod
is running. The name of the ConfigMap should be included as `--function-pod-template`, in the
command line arguments in the pod specification of the function runner.

```yaml
...
spec:
      serviceAccountName: porch-fn-runner
      containers:
        - name: function-runner
          image: gcr.io/example-google-project-id/porch-function-runner:latest
          imagePullPolicy: IfNotPresent
          command:
            - /server
            - --config=/config.yaml
            - --functions=/functions
            - --pod-namespace=porch-fn-system
            - --function-pod-template=kpt-function-eval-pod-template 
          env:
            - name: WRAPPER_SERVER_IMAGE
              value: gcr.io/example-google-project-id/porch-wrapper-server:latest
          ports:
            - containerPort: 9445
          # Add grpc readiness probe to ensure the cache is ready
          readinessProbe:
            exec:
              command:
                - /grpc-health-probe
                - -addr
                - localhost:9445
...
```

Additionally, the porch-fn-runner pod requires `read` access to the pod template ConfigMap. Assuming
the porch-fn-runner pod is running in the porch-system namespace, the following Role and
RoleBindings need to be added to the Porch deployment manifests.

```yaml
kind: Role
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: porch-fn-runner
  namespace: porch-system
rules:
  - apiGroups: [""]
    resources: ["configmaps"]
    verbs: ["get", "list"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: porch-fn-runner
  namespace: porch-system
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: porch-fn-runner
subjects:
  - kind: ServiceAccount
    name: porch-fn-runner
```

## Example pod template

The pod template ConfigMap below matches the default behavior:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: kpt-function-eval-pod-template
data:
  template: |
    apiVersion: v1
    kind: Pod
    annotations:
      cluster-autoscaler.kubernetes.io/safe-to-evict: true
    spec:
      initContainers:
        - name: copy-wrapper-server
          image: docker.io/nephio/porch-wrapper-server:latest
          command: 
            - cp
            - -a
            - /wrapper-server/.
            - /wrapper-server-tools
          volumeMounts:
            - name: wrapper-server-tools
              mountPath: /wrapper-server-tools
      containers:
        - name: function
          image: image-replaced-by-kpt-func-image
          command: 
            - /wrapper-server-tools/wrapper-server
          volumeMounts:
            - name: wrapper-server-tools
              mountPath: /wrapper-server-tools
      volumes:
        - name: wrapper-server-tools
          emptyDir: {}
  serviceTemplate: |
    apiVersion: v1
    kind: Service
    spec:
      ports:
      - port: 9446
        protocol: TCP
        targetPort: 9446
      selector:
        fn.kpt.dev/image: to-be-replaced
      type: ClusterIP
```
