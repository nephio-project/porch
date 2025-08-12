### Function Pod Template

In order to leverage custom manifests for Pod and frontend Service of Function Pods created by Function Runner, the following additional Kubernetes Resource Manifests (KRM) need to be provisioned in the Porch environment

* A ConfigMap containing 2 data elements: a) a KRM of type Pod under the `template` key and b) a KRM of type Service under `serviceTemplate` key
* A Kubernetes Role providing read access to resource type ConfigMap in the porch-system namespace
* A Kubernetes RoleBinding, binding to the above listed Role to the ServiceAccount (porch-fn-runner) used by Function Runner Pod

All of the above KRMs are predefined in `deployment.yaml` file present in this folder.

### How to enable Function Pod Template use by Function Runner

* Apply the [deployment.yaml manifest](deployment.yaml) from this directory

```
kubectl apply -f deployment.yaml
```

* Add an additional argument `--function-pod-template` in command section of function-runner deployment instructing it to use the Function Pod Template ConfigMap, as shown below

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

After the function-runner Pods restart, they will start using the Pod and Service templates from ConfigMap.
