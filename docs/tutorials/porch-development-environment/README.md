# Setting up a development environment for Porch

This tutorial gives short instructions on how to set up a development environment for Porch. It outlines the steps to get a [kind](https://kind.sigs.k8s.io/) cluster up
and running to which a Porch instance running in Visual Studio Code can connect to and interact with.

> **_NOTE:_**  The code itself can be run on a remote VM and we can use the [VSCode Remote SSH](https://marketplace.visualstudio.com/items?itemName=ms-vscode-remote.remote-ssh) plugin to connect to it as our Dev environment.

<br>

# Setup kind with MetalLB and Gitea

Follow steps 1-5 inclusive of the [Starting with Porch](https://github.com/nephio-project/porch/tree/main/docs/tutorials/starting-with-porch) tutorial. You now have the Kind cluster `management` running with Gitea installed on it. Gitea has the repository `management` defined.

> **_NOTE:_** This [setup script](bin/setup.sh) automates steps 1-5 of the Starting with Porch tutorial. You may need to adapt this script to your local environment and also have [pre requisites](https://github.com/nephio-project/porch/tree/main/docs/tutorials/starting-with-porch#prerequisites) installed on the target machine.

> **_NOTE:_** This [cleardown script script](bin/cleardown.sh) clears everything down by deleting the `management` Kind cluster. USE WITH CARE.

Switch to use the kind-management context if necessary:
```
kubectl config use-context kind-management
```


You can reach the Gitea web interface on the address reported by the following command:
```
kubectl get svc -n gitea gitea        
```
Sample output:
```
NAME    TYPE           CLUSTER-IP      EXTERNAL-IP      PORT(S)                       AGE
gitea   LoadBalancer   10.197.10.118   172.18.255.200   22:31260/TCP,3000:31012/TCP   8m35s
```

<br>

# Install the Porch function runner

The Porch server requires that the Porch function runner is executing. To install the Porch function runner on the Kind management cluster, execute the following commands:

```
kubectl apply -f https://raw.githubusercontent.com/nephio-project/catalog/main/nephio/core/porch/1-namespace.yaml
kubectl apply -f https://raw.githubusercontent.com/nephio-project/catalog/main/nephio/core/porch/2-function-runner.yaml

kubectl wait --namespace porch-system \
                --for=condition=ready pod \
                --selector=app=function-runner \
                --timeout=300s
```

The Porch function runner should now be executing:

```
kubectl get pod -n porch-system --selector=app=function-runner
```
Sample output:
```
NAME                              READY   STATUS    RESTARTS   AGE
function-runner-67d4c7c7b-7wm97   1/1     Running   0          16m
function-runner-67d4c7c7b-czvvq   1/1     Running   0          16m
```

Expose the `function-runner` service so that the Porch server running in Visual Studio Code can reach it. Patch the service type from `ClusterIP` to `LoadBalancer`:

```
kubectl patch svc -n porch-system function-runner -p '{"spec": {"type": "LoadBalancer"}}'
```

Check that the `function-runner` service has been assigned an external IP address:
```
kubectl get svc -n porch-system function-runner
```
Sample output:
```
NAME              TYPE           CLUSTER-IP       EXTERNAL-IP      PORT(S)          AGE
function-runner   LoadBalancer   10.197.168.148   172.18.255.201   9445:31794/TCP   22m
```
<br>

# Install the Porch CRDs

The Custom Resource Definitions can be applied to the cluster from the upstream porch kpt pkg as follows:

```
kubectl apply -f https://raw.githubusercontent.com/nephio-project/catalog/main/nephio/core/porch/0-packagerevs.yaml
kubectl apply -f https://raw.githubusercontent.com/nephio-project/catalog/main/nephio/core/porch/0-packagevariants.yaml
kubectl apply -f https://raw.githubusercontent.com/nephio-project/catalog/main/nephio/core/porch/0-packagevariantsets.yaml
kubectl apply -f https://raw.githubusercontent.com/nephio-project/catalog/main/nephio/core/porch/0-repositories.yaml
```

Check the entries:
```
kubectl get crd | grep porch
```
Sample output:
```
packagerevs.config.porch.kpt.dev          2024-03-11T15:07:12Z
packagevariants.config.porch.kpt.dev      2024-03-11T15:07:12Z
packagevariantsets.config.porch.kpt.dev   2024-03-11T15:07:13Z
repositories.config.porch.kpt.dev         2024-03-11T15:07:14Z
```
These new `resources` are now also visible wihtin the kubernetes api-resources:
```
kubectl api-resources | grep -i porch
```
Sample output:
```
packagerevs                                    config.porch.kpt.dev/v1alpha1     true         PackageRev
packagevariants                                config.porch.kpt.dev/v1alpha1     true         PackageVariant
packagevariantsets                             config.porch.kpt.dev/v1alpha2     true         PackageVariantSet
repositories                                   config.porch.kpt.dev/v1alpha1     true         Repository

```

<br>

# Deploy the porch APIService resources 

The Porch api server requires that the following resources are defined in the K8S cluster where it is executed:

- A `porch-system` namespace
- An APIService called `apiservice.apiregistration.k8s.io/v1alpha1.porch.kpt.dev`
- A `service.api` service to route the API Service requests. 

Slight differences in docker networking require a secific setup depending on the host OS.

## Mac OS example

Docker networking on Mac allows traffic to be routed via a default DNS name `host.docker.internal`, which is not available on Linux.

A sample configuration is available at `deployments/local/localconfig.yaml`

Apply the KRM:
```
kubectl apply -f https://raw.githubusercontent.com/nephio-project/porch/main/deployments/local/localconfig.yaml
```
Verify that the resources have been created
```
kubectl api-resources | grep -i porch
functions                                      config.porch.kpt.dev/v1alpha1     true         Function
packagerevs                                    config.porch.kpt.dev/v1alpha1     true         PackageRev
packagevariants                                config.porch.kpt.dev/v1alpha1     true         PackageVariant
packagevariantsets                             config.porch.kpt.dev/v1alpha2     true         PackageVariantSet
repositories                                   config.porch.kpt.dev/v1alpha1     true         Repository
```
## Linux OS example

Linux docker networking between `kind` clusters and the host processes require the traffic to be routed through the default docker bridge. See [here](https://github.com/kubernetes-sigs/kind/issues/1200#issuecomment-1532192361) for more details.

Apply the following resources:
```
kubectl apply -f - <<EOF
apiVersion: v1
kind: Namespace
metadata:
  name: porch-system
spec: {}
---
apiVersion: apiregistration.k8s.io/v1
kind: APIService
metadata:
  name: v1alpha1.porch.kpt.dev
spec:
  insecureSkipTLSVerify: true
  group: porch.kpt.dev
  groupPriorityMinimum: 1000
  versionPriority: 15
  service:
    name: api
    namespace: porch-system
    port: 9443
  version: v1alpha1

---
apiVersion: v1
kind: Endpoints
metadata:
  name: api
  namespace: porch-system
subsets:
  - addresses:
      - ip: 172.17.0.1 # docker0 bridge ip
    ports:
      - appProtocol: https
        port: 9443
        protocol: TCP
---
apiVersion: v1
kind: Service
metadata:
  name: api
  namespace: porch-system
spec:
  ports:
    - protocol: TCP
      appProtocol: https
      port: 9443
      targetPort: 9443
EOF
```
The new APIService will be unavailable until we run the porch api server:
```
kubectl get apiservice v1alpha1.porch.kpt.dev
```
Sample output:
```
NAME                     SERVICE            AVAILABLE                      AGE
v1alpha1.porch.kpt.dev   porch-system/api   False (FailedDiscoveryCheck)   7m46s
```

<br>

# Configure VSCode to run the Porch (api)server

From the root of your checked out Porch repo.

Edit your local `.vscode.launch.json` file as follows:
1. Change the `--kubeconfig` value to point at your management cluster configuration file.
2. Change the `--function-runner` IP address to the external IP of the function runner service running in the `management` cluster.
3. You can alternatively specify `KUBECONFIG` in an `env` section of the configuration instead of using the `--kubeconfig` flag.

```
        {
            "name": "Launch Server",
            "type": "go",
            "request": "launch",
            "mode": "auto",
            "program": "${workspaceFolder}/cmd/porch/main.go",
            "args": [
                "--secure-port=9443",
                "--v=7",
                "--standalone-debug-mode",
                "--kubeconfig=${userHome}/.kube/kind-management-config",
                "--cache-directory=${workspaceFolder}/.cache",
                "--function-runner=172.18.255.201:9445"
            ],
            "cwd": "${workspaceFolder}"
        },
```

You can now launch the Porch server locally in VSCode by selecting the "Launch Server" configuration on the VSCode "Run and Debug" window. for
more information please refer to the [VSCode debugging documentation](https://code.visualstudio.com/docs/editor/debugging).

<details closed>
<summary>Sample output</summary>

```
kubectl patch svc -n porch-system function-runner -p '{"spec": {"type": "LoadBalancer"}}'Starting: /home/ubuntu/go/bin/dlv dap --listen=127.0.0.1:40695 --log-dest=3 from /home/ubuntu/porch/cmd/porch
DAP server listening at: 127.0.0.1:40695
Type 'dlv help' for list of commands.
I0311 15:53:26.802270 2101307 dynamic_serving_content.go:113] "Loaded a new cert/key pair" name="serving-cert::apiserver.local.config/certificates/apiserver.crt::apiserver.local.config/certificates/apiserver.key"
W0311 15:53:26.963694 2101307 authentication.go:339] No authentication-kubeconfig provided in order to lookup client-ca-file in configmap/extension-apiserver-authentication in kube-system, so client certificate authentication won't work.
W0311 15:53:26.963716 2101307 authentication.go:363] No authentication-kubeconfig provided in order to lookup requestheader-client-ca-file in configmap/extension-apiserver-authentication in kube-system, so request-header client certificate authentication won't work.
W0311 15:53:26.963878 2101307 recommended.go:152] Neither kubeconfig is provided nor service-account is mounted, so APIPriorityAndFairness will be disabled
I0311 15:53:26.963942 2101307 maxinflight.go:140] "Initialized nonMutatingChan" len=400
I0311 15:53:26.963953 2101307 maxinflight.go:146] "Initialized mutatingChan" len=200
I0311 15:53:26.963979 2101307 timing_ratio_histogram.go:202] "TimingRatioHistogramVec.NewForLabelValuesSafe hit the inefficient case" fqName="apiserver_flowcontrol_read_vs_write_current_requests" labelValues=[executing readOnly]
I0311 15:53:26.963990 2101307 timing_ratio_histogram.go:202] "TimingRatioHistogramVec.NewForLabelValuesSafe hit the inefficient case" fqName="apiserver_flowcontrol_read_vs_write_current_requests" labelValues=[executing mutating]
I0311 15:53:26.964000 2101307 maxinflight.go:117] "Set denominator for readonly requests" limit=400
I0311 15:53:26.964006 2101307 maxinflight.go:121] "Set denominator for mutating requests" limit=200
I0311 15:53:26.964030 2101307 config.go:762] Not requested to run hook priority-and-fairness-config-consumer
I0311 15:53:26.966686 2101307 loader.go:373] Config loaded from file:  /home/ubuntu/.kube/kind-management-config
I0311 15:53:26.967652 2101307 round_trippers.go:463] GET https://127.0.0.1:31000/api?timeout=32s
I0311 15:53:26.967666 2101307 round_trippers.go:469] Request Headers:
I0311 15:53:26.967672 2101307 round_trippers.go:473]     Accept: application/json;g=apidiscovery.k8s.io;v=v2beta1;as=APIGroupDiscoveryList,application/json
I0311 15:53:26.967677 2101307 round_trippers.go:473]     User-Agent: __debug_bin3534874763/v0.0.0 (linux/amd64) kubernetes/$Format
I0311 15:53:26.975046 2101307 round_trippers.go:574] Response Status: 200 OK in 7 milliseconds
I0311 15:53:26.975945 2101307 round_trippers.go:463] GET https://127.0.0.1:31000/apis?timeout=32s
I0311 15:53:26.975958 2101307 round_trippers.go:469] Request Headers:
I0311 15:53:26.975964 2101307 round_trippers.go:473]     Accept: application/json;g=apidiscovery.k8s.io;v=v2beta1;as=APIGroupDiscoveryList,application/json
I0311 15:53:26.975968 2101307 round_trippers.go:473]     User-Agent: __debug_bin3534874763/v0.0.0 (linux/amd64) kubernetes/$Format
I0311 15:53:26.976802 2101307 round_trippers.go:574] Response Status: 200 OK in 0 milliseconds
I0311 15:53:26.979575 2101307 loader.go:373] Config loaded from file:  /home/ubuntu/.kube/kind-management-config
I0311 15:53:26.979853 2101307 grpcruntime.go:41] Dialing grpc function runner "172.18.255.201:9445"
I0311 15:53:26.979897 2101307 clientconn.go:318] "[core] [Channel #1] Channel created\n"
I0311 15:53:26.979924 2101307 logging.go:43] "[core] [Channel #1] original dial target is: \"172.18.255.201:9445\"\n"
I0311 15:53:26.979941 2101307 logging.go:43] "[core] [Channel #1] dial target \"172.18.255.201:9445\" parse failed: parse \"172.18.255.201:9445\": first path segment in URL cannot contain colon\n"
I0311 15:53:26.979951 2101307 logging.go:43] "[core] [Channel #1] fallback to scheme \"passthrough\"\n"
I0311 15:53:26.979974 2101307 logging.go:43] "[core] [Channel #1] parsed dial target is: {URL:{Scheme:passthrough Opaque: User: Host: Path:/172.18.255.201:9445 RawPath: OmitHost:false ForceQuery:false RawQuery: Fragment: RawFragment:}}\n"
I0311 15:53:26.979990 2101307 logging.go:43] "[core] [Channel #1] Channel authority set to \"172.18.255.201:9445\"\n"
I0311 15:53:26.980145 2101307 logging.go:43] "[core] [Channel #1] Resolver state updated: {\n  \"Addresses\": [\n    {\n      \"Addr\": \"172.18.255.201:9445\",\n      \"ServerName\": \"\",\n      \"Attributes\": null,\n      \"BalancerAttributes\": null,\n      \"Metadata\": null\n    }\n  ],\n  \"Endpoints\": [\n    {\n      \"Addresses\": [\n        {\n          \"Addr\": \"172.18.255.201:9445\",\n          \"ServerName\": \"\",\n          \"Attributes\": null,\n          \"BalancerAttributes\": null,\n          \"Metadata\": null\n        }\n      ],\n      \"Attributes\": null\n    }\n  ],\n  \"ServiceConfig\": null,\n  \"Attributes\": null\n} (resolver returned new addresses)\n"
I0311 15:53:26.980192 2101307 logging.go:43] "[core] [Channel #1] Channel switches to new LB policy \"pick_first\"\n"
I0311 15:53:26.980251 2101307 pickfirst.go:141] "[core] [pick-first-lb 0xc001813980] Received new config {\n  \"shuffleAddressList\": false\n}, resolver state {\n  \"Addresses\": [\n    {\n      \"Addr\": \"172.18.255.201:9445\",\n      \"ServerName\": \"\",\n      \"Attributes\": null,\n      \"BalancerAttributes\": null,\n      \"Metadata\": null\n    }\n  ],\n  \"Endpoints\": [\n    {\n      \"Addresses\": [\n        {\n          \"Addr\": \"172.18.255.201:9445\",\n          \"ServerName\": \"\",\n          \"Attributes\": null,\n          \"BalancerAttributes\": null,\n          \"Metadata\": null\n        }\n      ],\n      \"Attributes\": null\n    }\n  ],\n  \"ServiceConfig\": null,\n  \"Attributes\": null\n}\n"
I0311 15:53:26.980284 2101307 clientconn.go:962] "[core] [Channel #1 SubChannel #2] Subchannel created\n"
I0311 15:53:26.980300 2101307 logging.go:43] "[core] [Channel #1] Channel Connectivity change to CONNECTING\n"
I0311 15:53:26.980365 2101307 logging.go:43] "[core] [Channel #1 SubChannel #2] Subchannel Connectivity change to CONNECTING\n"
I0311 15:53:26.980395 2101307 logging.go:43] "[core] [Channel #1 SubChannel #2] Subchannel picks a new address \"172.18.255.201:9445\" to connect\n"
I0311 15:53:26.980492 2101307 pickfirst.go:184] "[core] [pick-first-lb 0xc001813980] Received SubConn state update: 0xc001813bc0, {ConnectivityState:CONNECTING ConnectionError:<nil>}\n"
I0311 15:53:26.980803 2101307 logging.go:43] "[core] [Channel #1 SubChannel #2] Subchannel Connectivity change to READY\n"
I0311 15:53:26.980831 2101307 pickfirst.go:184] "[core] [pick-first-lb 0xc001813980] Received SubConn state update: 0xc001813bc0, {ConnectivityState:READY ConnectionError:<nil>}\n"
I0311 15:53:26.980845 2101307 logging.go:43] "[core] [Channel #1] Channel Connectivity change to READY\n"
I0311 15:53:26.994444 2101307 apiserver.go:297] Cert storage dir not provided, skipping webhook setup
I0311 15:53:26.994524 2101307 background.go:52] Background routine starting ...
I0311 15:53:26.996985 2101307 healthz.go:176] Installing health checkers for (/healthz): "ping","log","poststarthook/max-in-flight-filter","poststarthook/storage-object-count-tracker-hook"
I0311 15:53:26.997336 2101307 healthz.go:176] Installing health checkers for (/livez): "ping","log","poststarthook/max-in-flight-filter","poststarthook/storage-object-count-tracker-hook"
I0311 15:53:26.997732 2101307 healthz.go:176] Installing health checkers for (/readyz): "ping","log","poststarthook/max-in-flight-filter","poststarthook/storage-object-count-tracker-hook","shutdown"
I0311 15:53:26.998191 2101307 genericapiserver.go:484] MuxAndDiscoveryComplete has all endpoints registered and discovery information is complete
I0311 15:53:26.998572 2101307 dynamic_serving_content.go:132] "Starting controller" name="serving-cert::apiserver.local.config/certificates/apiserver.crt::apiserver.local.config/certificates/apiserver.key"
I0311 15:53:26.998666 2101307 tlsconfig.go:200] "Loaded serving cert" certName="serving-cert::apiserver.local.config/certificates/apiserver.crt::apiserver.local.config/certificates/apiserver.key" certDetail="\"localhost@1709900706\" [serving] validServingFor=[127.0.0.1,localhost,localhost] issuer=\"localhost-ca@1709900706\" (2024-03-08 11:25:05 +0000 UTC to 2025-03-08 11:25:05 +0000 UTC (now=2024-03-11 15:53:26.998640146 +0000 UTC))"
I0311 15:53:26.998865 2101307 named_certificates.go:53] "Loaded SNI cert" index=0 certName="self-signed loopback" certDetail="\"apiserver-loopback-client@1710172406\" [serving] validServingFor=[apiserver-loopback-client] issuer=\"apiserver-loopback-client-ca@1710172406\" (2024-03-11 14:53:26 +0000 UTC to 2025-03-11 14:53:26 +0000 UTC (now=2024-03-11 15:53:26.998844612 +0000 UTC))"
I0311 15:53:26.998890 2101307 secure_serving.go:210] Serving securely on [::]:9443
I0311 15:53:26.998906 2101307 genericapiserver.go:589] [graceful-termination] waiting for shutdown to be initiated
I0311 15:53:26.998920 2101307 tlsconfig.go:240] "Starting DynamicServingCertificateController"
I0311 15:53:27.333650 2101307 handler.go:143] porch-apiserver: GET "/apis/porch.kpt.dev/v1alpha1/packagerevisions" satisfied by gorestful with webservice /apis/porch.kpt.dev/v1alpha1
I0311 15:53:27.333969 2101307 round_trippers.go:463] GET https://127.0.0.1:31000/apis/config.porch.kpt.dev/v1alpha1/repositories
I0311 15:53:27.333982 2101307 round_trippers.go:469] Request Headers:
I0311 15:53:27.333990 2101307 round_trippers.go:473]     Accept: application/json, */*
I0311 15:53:27.333995 2101307 round_trippers.go:473]     User-Agent: __debug_bin3534874763/v0.0.0 (linux/amd64) kubernetes/$Format
I0311 15:53:27.335504 2101307 round_trippers.go:574] Response Status: 200 OK in 1 milliseconds
I0311 15:53:27.336224 2101307 httplog.go:132] "HTTP" verb="LIST" URI="/apis/porch.kpt.dev/v1alpha1/packagerevisions?limit=500&resourceVersion=0" latency="2.73421ms" userAgent="kube-controller-manager/v1.29.2 (linux/amd64) kubernetes/4b8e819/metadata-informers" audit-ID="2bcc1626-35fb-495b-a4cf-fd99a83c7689" srcIP="172.18.0.2:24183" resp=200
I0311 15:53:27.337502 2101307 handler.go:143] porch-apiserver: GET "/apis/porch.kpt.dev/v1alpha1/packagerevisions" satisfied by gorestful with webservice /apis/porch.kpt.dev/v1alpha1
I0311 15:53:27.337598 2101307 get.go:257] "Starting watch" path="/apis/porch.kpt.dev/v1alpha1/packagerevisions" resourceVersion="" labels="" fields="" timeout="9m23s"
I0311 15:53:27.337712 2101307 watchermanager.go:93] added watcher 0xc001a99080; there are now 1 active watchers and 1 slots
I0311 15:53:27.337793 2101307 round_trippers.go:463] GET https://127.0.0.1:31000/apis/config.porch.kpt.dev/v1alpha1/repositories
I0311 15:53:27.337803 2101307 round_trippers.go:469] Request Headers:
I0311 15:53:27.337809 2101307 round_trippers.go:473]     Accept: application/json, */*
I0311 15:53:27.337813 2101307 round_trippers.go:473]     User-Agent: __debug_bin3534874763/v0.0.0 (linux/amd64) kubernetes/$Format
I0311 15:53:27.338998 2101307 round_trippers.go:574] Response Status: 200 OK in 1 milliseconds
I0311 15:53:27.339083 2101307 watch.go:201] watch 0xc0025eb8f0: moving watch into streaming mode after sentAdd 0, sentBacklog 0, sentNewBacklog 0
I0311 15:53:27.995010 2101307 background.go:76] Starting watch ...
I0311 15:53:27.995248 2101307 round_trippers.go:463] GET https://127.0.0.1:31000/apis/config.porch.kpt.dev/v1alpha1/repositories?allowWatchBookmarks=true&watch=true
I0311 15:53:27.995260 2101307 round_trippers.go:469] Request Headers:
I0311 15:53:27.995268 2101307 round_trippers.go:473]     Accept: application/json, */*
I0311 15:53:27.995273 2101307 round_trippers.go:473]     User-Agent: __debug_bin3534874763/v0.0.0 (linux/amd64) kubernetes/$Format
I0311 15:53:27.996235 2101307 round_trippers.go:574] Response Status: 200 OK in 0 milliseconds
I0311 15:53:27.996299 2101307 background.go:88] Watch successfully started.
I0311 15:53:29.226891 2101307 handler.go:133] porch-apiserver: GET "/apis" satisfied by gorestful with webservice /apis
I0311 15:53:29.227219 2101307 httplog.go:132] "HTTP" verb="GET" URI="/apis" latency="433.918µs" userAgent="" audit-ID="8a1a3928-6cb3-4cc1-87b2-1650858cbe05" srcIP="172.18.0.2:24183" resp=406
I0311 15:53:29.227817 2101307 handler.go:143] porch-apiserver: GET "/apis/porch.kpt.dev/v1alpha1" satisfied by gorestful with webservice /apis/porch.kpt.dev/v1alpha1
I0311 15:53:29.228088 2101307 httplog.go:132] "HTTP" verb="GET" URI="/apis/porch.kpt.dev/v1alpha1" latency="394.332µs" userAgent="" audit-ID="7d0e929b-2f88-40de-8332-6070b1511f1e" srcIP="172.18.0.2:24183" resp=200
I0311 15:53:29.318292 2101307 handler.go:143] porch-apiserver: GET "/apis/porch.kpt.dev/v1alpha1" satisfied by gorestful with webservice /apis/porch.kpt.dev/v1alpha1
I0311 15:53:29.318395 2101307 handler.go:143] porch-apiserver: GET "/apis/porch.kpt.dev/v1alpha1" satisfied by gorestful with webservice /apis/porch.kpt.dev/v1alpha1
I0311 15:53:29.318402 2101307 handler.go:143] porch-apiserver: GET "/apis/porch.kpt.dev/v1alpha1" satisfied by gorestful with webservice /apis/porch.kpt.dev/v1alpha1
I0311 15:53:29.318466 2101307 httplog.go:132] "HTTP" verb="GET" URI="/apis/porch.kpt.dev/v1alpha1" latency="265.744µs" userAgent="Go-http-client/2.0" audit-ID="75639942-377e-4154-8415-68a76ef6f3d4" srcIP="172.18.0.2:6653" resp=200
I0311 15:53:29.318478 2101307 handler.go:143] porch-apiserver: GET "/apis/porch.kpt.dev/v1alpha1" satisfied by gorestful with webservice /apis/porch.kpt.dev/v1alpha1
I0311 15:53:29.318527 2101307 httplog.go:132] "HTTP" verb="GET" URI="/apis/porch.kpt.dev/v1alpha1" latency="197.304µs" userAgent="Go-http-client/2.0" audit-ID="ae8435b4-c8db-4660-86c5-19cd8305459e" srcIP="172.18.0.2:6653" resp=200
I0311 15:53:29.318562 2101307 httplog.go:132] "HTTP" verb="GET" URI="/apis/porch.kpt.dev/v1alpha1" latency="267.733µs" userAgent="Go-http-client/2.0" audit-ID="eda93660-81fa-4528-b30c-17769417f8b0" srcIP="172.18.0.2:6653" resp=200
I0311 15:53:29.318589 2101307 httplog.go:132] "HTTP" verb="GET" URI="/apis/porch.kpt.dev/v1alpha1" latency="163.598µs" userAgent="Go-http-client/2.0" audit-ID="df104da4-f2a8-4ac9-a18f-016efc24cd5d" srcIP="172.18.0.2:6653" resp=200
I0311 15:53:29.318593 2101307 handler.go:143] porch-apiserver: GET "/apis/porch.kpt.dev/v1alpha1" satisfied by gorestful with webservice /apis/porch.kpt.dev/v1alpha1
I0311 15:53:29.318746 2101307 httplog.go:132] "HTTP" verb="GET" URI="/apis/porch.kpt.dev/v1alpha1" latency="220.87µs" userAgent="Go-http-client/2.0" audit-ID="1b821621-376c-494c-8490-4646d04a88e0" srcIP="172.18.0.2:6653" resp=200
I0311 15:53:29.320659 2101307 secure_serving.go:296] http: TLS handshake error from 172.18.0.2:44304: EOF
I0311 15:53:29.320719 2101307 secure_serving.go:296] http: TLS handshake error from 172.18.0.2:11578: EOF
I0311 15:53:59.314770 2101307 handler.go:143] porch-apiserver: GET "/apis/porch.kpt.dev/v1alpha1" satisfied by gorestful with webservice /apis/porch.kpt.dev/v1alpha1
I0311 15:53:59.314797 2101307 handler.go:143] porch-apiserver: GET "/apis/porch.kpt.dev/v1alpha1" satisfied by gorestful with webservice /apis/porch.kpt.dev/v1alpha1
I0311 15:53:59.314774 2101307 handler.go:143] porch-apiserver: GET "/apis/porch.kpt.dev/v1alpha1" satisfied by gorestful with webservice /apis/porch.kpt.dev/v1alpha1
I0311 15:53:59.314774 2101307 handler.go:143] porch-apiserver: GET "/apis/porch.kpt.dev/v1alpha1" satisfied by gorestful with webservice /apis/porch.kpt.dev/v1alpha1
I0311 15:53:59.314996 2101307 httplog.go:132] "HTTP" verb="GET" URI="/apis/porch.kpt.dev/v1alpha1" latency="400.618µs" userAgent="Go-http-client/2.0" audit-ID="82ea0403-a6b2-4222-abb4-de91f0d26a0c" srcIP="172.18.0.2:6653" resp=200
I0311 15:53:59.315027 2101307 httplog.go:132] "HTTP" verb="GET" URI="/apis/porch.kpt.dev/v1alpha1" latency="394.241µs" userAgent="Go-http-client/2.0" audit-ID="3e956684-e5aa-48ab-a2c3-fb896ef80017" srcIP="172.18.0.2:6653" resp=200
I0311 15:53:59.314996 2101307 httplog.go:132] "HTTP" verb="GET" URI="/apis/porch.kpt.dev/v1alpha1" latency="368.308µs" userAgent="Go-http-client/2.0" audit-ID="6f327a77-a5bb-4eec-b632-8ef9300d7472" srcIP="172.18.0.2:6653" resp=200
I0311 15:53:59.314777 2101307 handler.go:143] porch-apiserver: GET "/apis/porch.kpt.dev/v1alpha1" satisfied by gorestful with webservice /apis/porch.kpt.dev/v1alpha1
I0311 15:53:59.315181 2101307 httplog.go:132] "HTTP" verb="GET" URI="/apis/porch.kpt.dev/v1alpha1" latency="587.412µs" userAgent="Go-http-client/2.0" audit-ID="ab5efec5-0f49-41fb-a058-b64d96933437" srcIP="172.18.0.2:6653" resp=200
I0311 15:53:59.315200 2101307 httplog.go:132] "HTTP" verb="GET" URI="/apis/porch.kpt.dev/v1alpha1" latency="594.044µs" userAgent="Go-http-client/2.0" audit-ID="eecef25f-6825-4be7-94c9-86cc7e1966d4" srcIP="172.18.0.2:6653" resp=200
I0311 15:54:00.364529 2101307 handler.go:153] porch-apiserver: GET "/openapi/v2" satisfied by nonGoRestful
I0311 15:54:00.364560 2101307 pathrecorder.go:241] porch-apiserver: "/openapi/v2" satisfied by exact match
I0311 15:54:00.380302 2101307 handler.go:153] porch-apiserver: GET "/openapi/v2" satisfied by nonGoRestful
I0311 15:54:00.380326 2101307 pathrecorder.go:241] porch-apiserver: "/openapi/v2" satisfied by exact match
I0311 15:54:00.380694 2101307 httplog.go:132] "HTTP" verb="GET" URI="/openapi/v2" latency="16.285147ms" userAgent="" audit-ID="35d93c8e-31a0-4b5e-8f5b-6bc76760b905" srcIP="172.18.0.2:24183" resp=304
I0311 15:54:00.380740 2101307 httplog.go:132] "HTTP" verb="GET" URI="/openapi/v2" latency="540.523µs" userAgent="" audit-ID="b313e554-539d-4e3d-b4e0-223c2baf5a48" srcIP="172.18.0.2:24183" resp=304
I0311 15:54:26.995316 2101307 background.go:115] Background task 2024-03-11 15:54:26.995287314 +0000 UTC m=+60.349999574
I0311 15:54:26.995356 2101307 background.go:188] background-refreshing repositories
I0311 15:54:26.995487 2101307 round_trippers.go:463] GET https://127.0.0.1:31000/apis/config.porch.kpt.dev/v1alpha1/repositories
I0311 15:54:26.995496 2101307 round_trippers.go:469] Request Headers:
I0311 15:54:26.995504 2101307 round_trippers.go:473]     Accept: application/json, */*
I0311 15:54:26.995509 2101307 round_trippers.go:473]     User-Agent: __debug_bin3534874763/v0.0.0 (linux/amd64) kubernetes/$Format
I0311 15:54:26.997313 2101307 round_trippers.go:574] Response Status: 200 OK in 1 milliseconds
```

</details>

Check that the apiservice is now Ready:
```
kubectl get apiservice v1alpha1.porch.kpt.dev
```
Sample output:
```
NAME                     SERVICE            AVAILABLE   AGE
v1alpha1.porch.kpt.dev   porch-system/api   True        18m
```


Check the porch api-resources:

We should now also have the `porch.kpt.dev/v1alpha1` resources available
```
kubectl api-resources | grep porch
```
Sample output:
```
...

functions                                      porch.kpt.dev/v1alpha1            true         Function
packagerevisionresources                       porch.kpt.dev/v1alpha1            true         PackageRevisionResources
packagerevisions                               porch.kpt.dev/v1alpha1            true         PackageRevision
packages                                       porch.kpt.dev/v1alpha1            true         Package

```

Check to ensure that the apiserver is serving requests:
```
curl https://localhost:9443/apis/porch.kpt.dev/v1alpha1 -k
```

<details closed>
<summary>Sample output</summary>

```json
{
  "kind": "APIResourceList",
  "apiVersion": "v1",
  "groupVersion": "porch.kpt.dev/v1alpha1",
  "resources": [
    {
      "name": "functions",
      "singularName": "",
      "namespaced": true,
      "kind": "Function",
      "verbs": [
        "get",
        "list"
      ]
    },
    {
      "name": "packagerevisionresources",
      "singularName": "",
      "namespaced": true,
      "kind": "PackageRevisionResources",
      "verbs": [
        "get",
        "list",
        "patch",
        "update"
      ]
    },
    {
      "name": "packagerevisions",
      "singularName": "",
      "namespaced": true,
      "kind": "PackageRevision",
      "verbs": [
        "create",
        "delete",
        "get",
        "list",
        "patch",
        "update",
        "watch"
      ]
    },
    {
      "name": "packagerevisions/approval",
      "singularName": "",
      "namespaced": true,
      "kind": "PackageRevision",
      "verbs": [
        "get",
        "patch",
        "update"
      ]
    },
    {
      "name": "packages",
      "singularName": "",
      "namespaced": true,
      "kind": "Package",
      "verbs": [
        "create",
        "delete",
        "get",
        "list",
        "patch",
        "update"
      ]
    }
  ]
}
```
</details>


<br>

# Create Repositories using your local Porch server

To connect Porch to Gitea, follow [step 7 in the Starting with Porch](https://github.com/nephio-project/porch/tree/main/docs/tutorials/starting-with-porch#Connect-the-Gitea-repositories-to-Porch) tutorial to create the repositories in Porch.

You will notice logging messages in VSCode when you run the command `kubectl apply -f porch-repositories.yaml` command.

You can check that your locally running Porch server has created the repositories by running the `porchctl` command:

```
porchctl repo get -A
```
Sample output:
```
NAME                  TYPE   CONTENT   DEPLOYMENT   READY   ADDRESS
external-blueprints   git    Package   false        True    https://github.com/nephio-project/free5gc-packages.git
management            git    Package   false        True    http://172.18.255.200:3000/nephio/management.git
```

You can also check the repositories using kubectl.

```
kubectl get  repositories -n porch-demo
```

Sample output:
```
NAME                  TYPE   CONTENT   DEPLOYMENT   READY   ADDRESS
external-blueprints   git    Package   false        True    https://github.com/nephio-project/free5gc-packages.git
management            git    Package   false        True    http://172.18.255.200:3000/nephio/management.git
```

You now have a locally running Porch (api)server. Happy developing!