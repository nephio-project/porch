### Function Pod Template

In order to leverage custom manifests for Pod and frontend Service of Function Pods created by Function Runner, following additional k8s resources needs to be provisioned in Porch environment

* A ConfigMap containing 2 data elements: a) Kubernetes manifest of Function Pod under template key and b) manifest of frontend Service under serviceTemplate key
* A Role providing read access to ConfigMap in porch-system namespace
* A RoleBinding mapping above listed role to ServiceAccount used by Function Runner pod

All of above 3 k8s resources are added in deployment.yaml file present in this folder.

### How to enable Function Pod Template use by Function Runner

* Apply the [deployment.yaml manifest](deployment.yaml) from this directory

```
kubectl apply -f deployment.yaml
```

* Add additional argument --function-pod-template in command section of function-runner deployment instructing it to use Function Pod Template configmap, as shown below

```
kubectl edit deployment -n porch-system function-runner
```

```
          command:
            - /server
            - --config=/config.yaml
            - --functions=/functions
            - --pod-namespace=porch-fn-system
            - --function-pod-template=kpt-function-eval-pod-template
            - --max-request-body-size=6291456 # Keep this in sync with porch-server's corresponding argument
```

After function-runner pods restart, they will start using the Pod and Service templates from config map.
