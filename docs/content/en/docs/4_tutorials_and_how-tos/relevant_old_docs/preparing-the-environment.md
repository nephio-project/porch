---
title: "Preparing the Environment"
type: docs
weight: 2
description: "A tutorial to preparing the environment for Porch"
---

<div style="border: 1px solid red; background-color: #ffe6e6; color: #b30000; padding: 1em; margin-bottom: 1em;">
  <strong>⚠️ Outdated Notice:</strong> This page refers to an older version of the documentation. This content has simply been moved into its relevant new section here and must be checked, modified, rewritten, updated, or removed entirely.
</div>

## Exploring the Porch resources

We have configured three repositories in Porch:

```bash
kubectl get repositories -n porch-demo
NAME                  TYPE   CONTENT   DEPLOYMENT   READY   ADDRESS
edge1                 git    Package   true         True    http://172.18.255.200:3000/nephio/edge1.git
external-blueprints   git    Package   false        True    https://github.com/nephio-project/free5gc-packages.git
management            git    Package   false        True    http://172.18.255.200:3000/nephio/management.git
```

A repository is a CR of the Porch Repository CRD. You can examine the *repositories.config.porch.kpt.dev* CRD with
either of the following commands (both of which are rather verbose):

```bash
kubectl get crd -n porch-system repositories.config.porch.kpt.dev -o yaml
kubectl describe crd -n porch-system repositories.config.porch.kpt.dev 
```

You can examine any other CRD using the commands above and changing the CRD name/namespace.

The full list of Nephio CRDs is as below:

```bash
kubectl api-resources --api-group=porch.kpt.dev         
NAME                       SHORTNAMES   APIVERSION               NAMESPACED   KIND
packagerevisionresources                porch.kpt.dev/v1alpha1   true         PackageRevisionResources
packagerevisions                        porch.kpt.dev/v1alpha1   true         PackageRevision
packages                                porch.kpt.dev/v1alpha1   true         Package
```

The PackageRevision CRD is used to keep track of revision (or version) of each package found in the repositories.

```bash
kubectl get packagerevision -n porch-demo
NAME                                                           PACKAGE              WORKSPACENAME   REVISION   LATEST   LIFECYCLE   REPOSITORY
external-blueprints-922121d0bcdd56bfa8cae6c375720e2b5f358ab0   free5gc-cp           main            main       false    Published   external-blueprints
external-blueprints-dabbc422fdf0b8e5942e767d929b524e25f7eef9   free5gc-cp           v1              v1         true     Published   external-blueprints
external-blueprints-716aae722092dbbb9470e56079b90ad76ec8f0d5   free5gc-operator     main            main       false    Published   external-blueprints
external-blueprints-d65dc89f7a2472650651e9aea90edfcc81a9afc6   free5gc-operator     v1              v1         false    Published   external-blueprints
external-blueprints-9fee880e8fa52066f052c9cae7aac2e2bc1b5a54   free5gc-operator     v2              v2         false    Published   external-blueprints
external-blueprints-91d60ee31d2d0a1a6d5f1807593d5419434accd3   free5gc-operator     v3              v3         false    Published   external-blueprints
external-blueprints-21f19a0641cf520e7dc6268e64c58c2c30c27036   free5gc-operator     v4              v4         false    Published   external-blueprints
external-blueprints-bf2e7522ee92680bd49571ab309e3f61320cf36d   free5gc-operator     v5              v5         true     Published   external-blueprints
external-blueprints-c1b9ecb73118e001ab1d1213e6a2c94ab67a0939   free5gc-upf          main            main       false    Published   external-blueprints
external-blueprints-5d48b1516e7b1ea15830ffd76b230862119981bd   free5gc-upf          v1              v1         true     Published   external-blueprints
external-blueprints-ed97798b46b36d135cf23d813eccad4857dff90f   pkg-example-amf-bp   main            main       false    Published   external-blueprints
external-blueprints-ed744bfdf4a4d15d4fcf3c46fde27fd6ac32d180   pkg-example-amf-bp   v1              v1         false    Published   external-blueprints
external-blueprints-5489faa80782f91f1a07d04e206935d14c1eb24c   pkg-example-amf-bp   v2              v2         false    Published   external-blueprints
external-blueprints-16e2255bd433ef532684a3c1434ae0bede175107   pkg-example-amf-bp   v3              v3         false    Published   external-blueprints
external-blueprints-7689cc6c953fa83ea61283983ce966dcdffd9bae   pkg-example-amf-bp   v4              v4         false    Published   external-blueprints
external-blueprints-caff9609883eea7b20b73b7425e6694f8eb6adc3   pkg-example-amf-bp   v5              v5         true     Published   external-blueprints
external-blueprints-00b6673c438909975548b2b9f20c2e1663161815   pkg-example-smf-bp   main            main       false    Published   external-blueprints
external-blueprints-4f7dfbede99dc08f2b5144ca550ca218109c52f2   pkg-example-smf-bp   v1              v1         false    Published   external-blueprints
external-blueprints-3d9ab8f61ce1d35e264d5719d4b3c0da1ab02328   pkg-example-smf-bp   v2              v2         false    Published   external-blueprints
external-blueprints-2006501702e105501784c78be9e7d57e426d85e8   pkg-example-smf-bp   v3              v3         false    Published   external-blueprints
external-blueprints-c97ed7c13b3aa47cb257217f144960743aec1253   pkg-example-smf-bp   v4              v4         false    Published   external-blueprints
external-blueprints-3bd78e46b014dac5cc0c58788c1820d043d61569   pkg-example-smf-bp   v5              v5         true     Published   external-blueprints
external-blueprints-c3f660848d9d7a4df5481ec2e06196884778cd84   pkg-example-upf-bp   main            main       false    Published   external-blueprints
external-blueprints-4cb00a17c1ee2585d6c187ba4d0211da960c0940   pkg-example-upf-bp   v1              v1         false    Published   external-blueprints
external-blueprints-5903efe295026124e6fea926df154a72c5bd1ea9   pkg-example-upf-bp   v2              v2         false    Published   external-blueprints
external-blueprints-16142d8d23c1b8e868a9524a1b21634c79b432d5   pkg-example-upf-bp   v3              v3         false    Published   external-blueprints
external-blueprints-60ef45bb8f55b63556e7467f16088325022a7ece   pkg-example-upf-bp   v4              v4         false    Published   external-blueprints
external-blueprints-7757966cc7b965f1b9372370a4b382c8375a2b40   pkg-example-upf-bp   v5              v5         true     Published   external-blueprints
```

The PackageRevisionResources resource is an API Aggregation resource that Porch uses to wrap the GET URL for the package
on its repository.

```bash
kubectl get packagerevisionresources  -n porch-demo
NAME                                                           PACKAGE              WORKSPACENAME   REVISION   REPOSITORY            FILES
external-blueprints-922121d0bcdd56bfa8cae6c375720e2b5f358ab0   free5gc-cp           main            main       external-blueprints   28
external-blueprints-dabbc422fdf0b8e5942e767d929b524e25f7eef9   free5gc-cp           v1              v1         external-blueprints   28
external-blueprints-716aae722092dbbb9470e56079b90ad76ec8f0d5   free5gc-operator     main            main       external-blueprints   14
external-blueprints-d65dc89f7a2472650651e9aea90edfcc81a9afc6   free5gc-operator     v1              v1         external-blueprints   11
external-blueprints-9fee880e8fa52066f052c9cae7aac2e2bc1b5a54   free5gc-operator     v2              v2         external-blueprints   11
external-blueprints-91d60ee31d2d0a1a6d5f1807593d5419434accd3   free5gc-operator     v3              v3         external-blueprints   14
external-blueprints-21f19a0641cf520e7dc6268e64c58c2c30c27036   free5gc-operator     v4              v4         external-blueprints   14
external-blueprints-bf2e7522ee92680bd49571ab309e3f61320cf36d   free5gc-operator     v5              v5         external-blueprints   14
external-blueprints-c1b9ecb73118e001ab1d1213e6a2c94ab67a0939   free5gc-upf          main            main       external-blueprints   6
external-blueprints-5d48b1516e7b1ea15830ffd76b230862119981bd   free5gc-upf          v1              v1         external-blueprints   6
external-blueprints-ed97798b46b36d135cf23d813eccad4857dff90f   pkg-example-amf-bp   main            main       external-blueprints   16
external-blueprints-ed744bfdf4a4d15d4fcf3c46fde27fd6ac32d180   pkg-example-amf-bp   v1              v1         external-blueprints   7
external-blueprints-5489faa80782f91f1a07d04e206935d14c1eb24c   pkg-example-amf-bp   v2              v2         external-blueprints   8
external-blueprints-16e2255bd433ef532684a3c1434ae0bede175107   pkg-example-amf-bp   v3              v3         external-blueprints   16
external-blueprints-7689cc6c953fa83ea61283983ce966dcdffd9bae   pkg-example-amf-bp   v4              v4         external-blueprints   16
external-blueprints-caff9609883eea7b20b73b7425e6694f8eb6adc3   pkg-example-amf-bp   v5              v5         external-blueprints   16
external-blueprints-00b6673c438909975548b2b9f20c2e1663161815   pkg-example-smf-bp   main            main       external-blueprints   17
external-blueprints-4f7dfbede99dc08f2b5144ca550ca218109c52f2   pkg-example-smf-bp   v1              v1         external-blueprints   8
external-blueprints-3d9ab8f61ce1d35e264d5719d4b3c0da1ab02328   pkg-example-smf-bp   v2              v2         external-blueprints   9
external-blueprints-2006501702e105501784c78be9e7d57e426d85e8   pkg-example-smf-bp   v3              v3         external-blueprints   17
external-blueprints-c97ed7c13b3aa47cb257217f144960743aec1253   pkg-example-smf-bp   v4              v4         external-blueprints   17
external-blueprints-3bd78e46b014dac5cc0c58788c1820d043d61569   pkg-example-smf-bp   v5              v5         external-blueprints   17
external-blueprints-c3f660848d9d7a4df5481ec2e06196884778cd84   pkg-example-upf-bp   main            main       external-blueprints   17
external-blueprints-4cb00a17c1ee2585d6c187ba4d0211da960c0940   pkg-example-upf-bp   v1              v1         external-blueprints   8
external-blueprints-5903efe295026124e6fea926df154a72c5bd1ea9   pkg-example-upf-bp   v2              v2         external-blueprints   8
external-blueprints-16142d8d23c1b8e868a9524a1b21634c79b432d5   pkg-example-upf-bp   v3              v3         external-blueprints   17
external-blueprints-60ef45bb8f55b63556e7467f16088325022a7ece   pkg-example-upf-bp   v4              v4         external-blueprints   17
external-blueprints-7757966cc7b965f1b9372370a4b382c8375a2b40   pkg-example-upf-bp   v5              v5         external-blueprints   17
```

Let's examine the *free5gc-cp v1* package.

The PackageRevision CR name for *free5gc-cp v1* is external-blueprints-dabbc422fdf0b8e5942e767d929b524e25f7eef9.

```bash
kubectl get packagerevision -n porch-demo external-blueprints-dabbc422fdf0b8e5942e767d929b524e25f7eef9 -o yaml
```

```yaml
apiVersion: porch.kpt.dev/v1alpha1
kind: PackageRevision
metadata:
  creationTimestamp: "2023-06-13T13:35:34Z"
  labels:
    kpt.dev/latest-revision: "true"
  name: external-blueprints-dabbc422fdf0b8e5942e767d929b524e25f7eef9
  namespace: porch-demo
  resourceVersion: 5fc9561dcd4b2630704c192e89887490e2ff3c61
  uid: uid:free5gc-cp:v1
spec:
  lifecycle: Published
  packageName: free5gc-cp
  repository: external-blueprints
  revision: v1
  workspaceName: v1
status:
  publishTimestamp: "2023-06-13T13:35:34Z"
  publishedBy: dnaleksandrov@gmail.com
  upstreamLock: {}
```

Getting the *PackageRevisionResources* pulls the package from its repository with each file serialized into a name-value
map of resources in it's spec.

<details>
<summary>Open this to see the command and the result</summary>

```bash
kubectl get packagerevisionresources -n porch-demo external-blueprints-dabbc422fdf0b8e5942e767d929b524e25f7eef9 -o yaml
```
```yaml
apiVersion: porch.kpt.dev/v1alpha1
kind: PackageRevisionResources
metadata:
  creationTimestamp: "2023-06-13T13:35:34Z"
  name: external-blueprints-dabbc422fdf0b8e5942e767d929b524e25f7eef9
  namespace: porch-demo
  resourceVersion: 5fc9561dcd4b2630704c192e89887490e2ff3c61
  uid: uid:free5gc-cp:v1
spec:
  packageName: free5gc-cp
  repository: external-blueprints
  resources:
    Kptfile: |
      apiVersion: kpt.dev/v1
      kind: Kptfile
      metadata:
        name: free5gc-cp
        annotations:
          config.kubernetes.io/local-config: "true"
      info:
        description: this package represents free5gc NFs, which are required to perform E2E conn testing
      pipeline:
        mutators:
          - image: gcr.io/kpt-fn/set-namespace:v0.4.1
            configPath: package-context.yaml
    README.md: "# free5gc-cp\n\n## Description\nPackage representing free5gc control
      plane NFs.\n\nPackage definition is based on [Towards5gs helm charts](https://github.com/Orange-OpenSource/towards5gs-helm),
      \nand service level configuration is preserved as defined there.\n\n### Network
      Functions (NFs)\n\nfree5gc project implements following NFs:\n\n\n| NF | Description
      | local-config |\n| --- | --- | --- |\n| AMF | Access and Mobility Management
      Function | true |\n| AUSF | Authentication Server Function | false |\n| NRF
      | Network Repository Function | false |\n| NSSF | Network Slice Selection Function
      | false |\n| PCF | Policy Control Function | false |\n| SMF | Session Management
      Function | true |\n| UDM | Unified Data Management | false |\n| UDR | Unified
      Data Repository | false |\n\nalso Database and Web UI is defined:\n\n| Service
      | Description | local-config |\n| --- | --- | --- |\n| mongodb | Database to
      store free5gc data | false |\n| webui | UI used to register UE | false |\n\nNote:
      `local-config: true` indicates that this resources won't be deployed to the
      workload cluster\n\n### Dependencies\n\n- `mongodb` requires `Persistent Volume`.
      We need to assure that dynamic PV provisioning will be available on the cluster\n-
      `NRF` should be running before other NFs will be instantiated\n    - all NFs
      packages contain `wait-nrf` init-container\n- `NRF` and `WEBUI` require DB\n
      \   - packages contain `wait-mongodb` init-container\n- `WEBUI` service is exposed
      as `NodePort` \n    - will be used to register UE on the free5gc side\n- Communication
      via `SBI` between NFs and communication with `mongodb` is defined using K8s
      `ClusterIP` services\n    - it forces you to deploy all NFs on a single cluster
      or consider including `service mesh` in a multi-cluster scenario\n\n## Usage\n\n###
      Fetch the package\n`kpt pkg get REPO_URI[.git]/PKG_PATH[@VERSION] free5gc-cp`\n\nDetails:
      https://kpt.dev/reference/cli/pkg/get/\n\n### View package content\n`kpt pkg
      tree free5gc-cp`\n\nDetails: https://kpt.dev/reference/cli/pkg/tree/\n\n###
      Apply the package\n```\nkpt live init free5gc-cp\nkpt live apply free5gc-cp
      --reconcile-timeout=2m --output=table\n```\n\nDetails: https://kpt.dev/reference/cli/live/\n\n"
    ausf/ausf-configmap.yaml: "---\napiVersion: v1\nkind: ConfigMap\nmetadata:\n  name:
      ausf-configmap\n  labels:\n    app.kubernetes.io/version: \"v3.1.1\"\n    app:
      free5gc\ndata:\n  ausfcfg.yaml: |\n    info:\n      version: 1.0.2\n      description:
      AUSF initial local configuration\n\n    configuration:\n      serviceNameList:\n
      \       - nausf-auth\n      \n      sbi:\n        scheme: http\n        registerIPv4:
      ausf-nausf  # IP used to register to NRF\n        bindingIPv4: 0.0.0.0      #
      IP used to bind the service\n        port: 80\n        tls:\n          key:
      config/TLS/ausf.key\n          pem: config/TLS/ausf.pem\n      \n      nrfUri:
      http://nrf-nnrf:8000\n      plmnSupportList:\n        - mcc: 208\n          mnc:
      93\n        - mcc: 123\n          mnc: 45\n      groupId: ausfGroup001\n      eapAkaSupiImsiPrefix:
      false\n\n    logger:\n      AUSF:\n        ReportCaller: false\n        debugLevel:
      info\n"
    ausf/ausf-deployment.yaml: "---\napiVersion: apps/v1\nkind: Deployment\nmetadata:\n
      \ name: free5gc-ausf\n  labels:\n    app.kubernetes.io/version: \"v3.1.1\"\n
      \   project: free5gc\n    nf: ausf\nspec:\n  replicas: 1\n  selector:\n    matchLabels:\n
      \     project: free5gc\n      nf: ausf\n  template:\n    metadata:\n      labels:\n
      \       project: free5gc\n        nf: ausf\n    spec:\n      initContainers:\n
      \     - name: wait-nrf\n        image: towards5gs/initcurl:1.0.0\n        env:\n
      \       - name: DEPENDENCIES\n          value: http://nrf-nnrf:8000\n        command:
      ['sh', '-c', 'set -x; for dependency in $DEPENDENCIES; do while [ $(curl --insecure
      --connect-timeout 1 -s -o /dev/null -w \"%{http_code}\" $dependency) -ne 200
      ]; do echo waiting for dependencies; sleep 1; done; done;']\n      \n      containers:\n
      \     - name: ausf\n        image: towards5gs/free5gc-ausf:v3.1.1\n        imagePullPolicy:
      IfNotPresent\n        securityContext:\n            {}\n        ports:\n        -
      containerPort: 80\n        command: [\"./ausf\"]\n        args: [\"-c\", \"../config/ausfcfg.yaml\"]\n
      \       env:\n          - name: GIN_MODE\n            value: release\n        volumeMounts:\n
      \       - mountPath: /free5gc/config/\n          name: ausf-volume\n        resources:\n
      \           limits:\n              cpu: 100m\n              memory: 128Mi\n
      \           requests:\n              cpu: 100m\n              memory: 128Mi\n
      \     dnsPolicy: ClusterFirst\n      restartPolicy: Always\n\n      volumes:\n
      \     - name: ausf-volume\n        projected:\n          sources:\n          -
      configMap:\n              name: ausf-configmap\n"
    ausf/ausf-service.yaml: |
      ---
      apiVersion: v1
      kind: Service
      metadata:
        name: ausf-nausf
        labels:
          app.kubernetes.io/version: "v3.1.1"
          project: free5gc
          nf: ausf
      spec:
        type: ClusterIP
        ports:
          - port: 80
            targetPort: 80
            protocol: TCP
            name: http
        selector:
          project: free5gc
          nf: ausf
    mongodb/dep-sts.yaml: "---\napiVersion: apps/v1\nkind: StatefulSet\nmetadata:\n
      \ name: mongodb\n  namespace: default\n  labels:\n    app.kubernetes.io/name:
      mongodb\n    app.kubernetes.io/instance: free5gc\n    app.kubernetes.io/component:
      mongodb\nspec:\n  serviceName: mongodb\n  updateStrategy:\n    type: RollingUpdate\n
      \ selector:\n    matchLabels:\n      app.kubernetes.io/name: mongodb\n      app.kubernetes.io/instance:
      free5gc\n      app.kubernetes.io/component: mongodb\n  template:\n    metadata:\n
      \     labels:\n        app.kubernetes.io/name: mongodb\n        app.kubernetes.io/instance:
      free5gc\n        app.kubernetes.io/component: mongodb\n    spec:\n      \n      serviceAccountName:
      mongodb\n      affinity:\n        podAffinity:\n        podAntiAffinity:\n          preferredDuringSchedulingIgnoredDuringExecution:\n
      \           - podAffinityTerm:\n                labelSelector:\n                  matchLabels:\n
      \                   app.kubernetes.io/name: mongodb\n                    app.kubernetes.io/instance:
      free5gc\n                    app.kubernetes.io/component: mongodb\n                namespaces:\n
      \                 - \"default\"\n                topologyKey: kubernetes.io/hostname\n
      \             weight: 1\n        nodeAffinity:\n          \n      securityContext:\n
      \       fsGroup: 1001\n        sysctls: []\n      containers:\n        - name:
      mongodb\n          image: docker.io/bitnami/mongodb:4.4.4-debian-10-r0\n          imagePullPolicy:
      \"IfNotPresent\"\n          securityContext:\n            runAsNonRoot: true\n
      \           runAsUser: 1001\n          env:\n            - name: BITNAMI_DEBUG\n
      \             value: \"false\"\n            - name: ALLOW_EMPTY_PASSWORD\n              value:
      \"yes\"\n            - name: MONGODB_SYSTEM_LOG_VERBOSITY\n              value:
      \"0\"\n            - name: MONGODB_DISABLE_SYSTEM_LOG\n              value:
      \"no\"\n            - name: MONGODB_ENABLE_IPV6\n              value: \"no\"\n
      \           - name: MONGODB_ENABLE_DIRECTORY_PER_DB\n              value: \"no\"\n
      \         ports:\n            - name: mongodb\n              containerPort:
      27017\n          livenessProbe:\n            exec:\n              command:\n
      \               - mongo\n                - --disableImplicitSessions\n                -
      --eval\n                - \"db.adminCommand('ping')\"\n            initialDelaySeconds:
      30\n            periodSeconds: 10\n            timeoutSeconds: 5\n            successThreshold:
      1\n            failureThreshold: 6\n          readinessProbe:\n            exec:\n
      \             command:\n                - bash\n                - -ec\n                -
      |\n                  mongo --disableImplicitSessions $TLS_OPTIONS --eval 'db.hello().isWritablePrimary
      || db.hello().secondary' | grep -q 'true'\n            initialDelaySeconds:
      5\n            periodSeconds: 10\n            timeoutSeconds: 5\n            successThreshold:
      1\n            failureThreshold: 6\n          resources:\n            limits:
      {}\n            requests: {}\n          volumeMounts:\n            - name: datadir\n
      \             mountPath: /bitnami/mongodb/data/db/\n              subPath: \n
      \     volumes:\n  volumeClaimTemplates:\n    - metadata:\n        name: datadir\n
      \     spec:\n        accessModes:\n          - \"ReadWriteOnce\"\n        resources:\n
      \         requests:\n            storage: \"6Gi\"\n"
    mongodb/serviceaccount.yaml: |
      ---
      apiVersion: v1
      kind: ServiceAccount
      metadata:
        name: mongodb
        namespace: default
        labels:
          app.kubernetes.io/name: mongodb
          app.kubernetes.io/instance: free5gc
      secrets:
        - name: mongodb
    mongodb/svc.yaml: |
      ---
      apiVersion: v1
      kind: Service
      metadata:
        name: mongodb
        namespace: default
        labels:
          app.kubernetes.io/name: mongodb
          app.kubernetes.io/instance: free5gc
          app.kubernetes.io/component: mongodb
      spec:
        type: ClusterIP
        ports:
          - name: mongodb
            port: 27017
            targetPort: mongodb
            nodePort: null
        selector:
          app.kubernetes.io/name: mongodb
          app.kubernetes.io/instance: free5gc
          app.kubernetes.io/component: mongodb
    namespace.yaml: |
      apiVersion: v1
      kind: Namespace
      metadata:
        name: example
        labels:
          pod-security.kubernetes.io/warn: "privileged"
          pod-security.kubernetes.io/audit: "privileged"
          pod-security.kubernetes.io/enforce: "privileged"
    nrf/nrf-configmap.yaml: "---\napiVersion: v1\nkind: ConfigMap\nmetadata:\n  name:
      nrf-configmap\n  labels:\n    app.kubernetes.io/version: \"v3.1.1\"\n    app:
      free5gc\ndata:\n  nrfcfg.yaml: |\n    info:\n      version: 1.0.1\n      description:
      NRF initial local configuration\n    \n    configuration:\n      MongoDBName:
      free5gc\n      MongoDBUrl: mongodb://mongodb:27017\n\n      serviceNameList:\n
      \       - nnrf-nfm\n        - nnrf-disc\n\n      sbi:\n        scheme: http\n
      \       registerIPv4: nrf-nnrf  # IP used to serve NFs or register to another
      NRF\n        bindingIPv4: 0.0.0.0    # IP used to bind the service\n        port:
      8000\n        tls:\n          key: config/TLS/nrf.key\n          pem: config/TLS/nrf.pem\n
      \     DefaultPlmnId:\n        mcc: 208\n        mnc: 93\n\n    logger:\n      NRF:\n
      \       ReportCaller: false\n        debugLevel: info\n"
    nrf/nrf-deployment.yaml: "---\napiVersion: apps/v1\nkind: Deployment\nmetadata:\n
      \ name: free5gc-nrf\n  labels:\n    app.kubernetes.io/version: \"v3.1.1\"\n
      \   project: free5gc\n    nf: nrf\nspec:\n  replicas: 1\n  selector:\n    matchLabels:\n
      \     project: free5gc\n      nf: nrf\n  template:\n    metadata:\n      labels:\n
      \       project: free5gc\n        nf: nrf\n    spec:\n      initContainers:\n
      \     - name: wait-mongo\n        image: busybox:1.32.0\n        env:\n        -
      name: DEPENDENCIES\n          value: mongodb:27017\n        command: [\"sh\",
      \"-c\", \"until nc -z $DEPENDENCIES; do echo waiting for the MongoDB; sleep
      2; done;\"]\n      containers:\n      - name: nrf\n        image: towards5gs/free5gc-nrf:v3.1.1\n
      \       imagePullPolicy: IfNotPresent\n        securityContext:\n            {}\n
      \       ports:\n        - containerPort: 8000\n        command: [\"./nrf\"]\n
      \       args: [\"-c\", \"../config/nrfcfg.yaml\"]\n        env: \n          -
      name: DB_URI\n            value: mongodb://mongodb/free5gc\n          - name:
      GIN_MODE\n            value: release\n        volumeMounts:\n        - mountPath:
      /free5gc/config/\n          name: nrf-volume\n        resources:\n            limits:\n
      \             cpu: 100m\n              memory: 128Mi\n            requests:\n
      \             cpu: 100m\n              memory: 128Mi\n        readinessProbe:\n
      \         initialDelaySeconds: 0\n          periodSeconds: 1\n          timeoutSeconds:
      1\n          failureThreshold:  40\n          successThreshold: 1\n          httpGet:\n
      \           scheme: \"HTTP\"\n            port: 8000\n        livenessProbe:\n
      \         initialDelaySeconds: 120\n          periodSeconds: 10\n          timeoutSeconds:
      10\n          failureThreshold: 3\n          successThreshold: 1\n          httpGet:\n
      \           scheme: \"HTTP\"\n            port: 8000\n      dnsPolicy: ClusterFirst\n
      \     restartPolicy: Always\n\n      volumes:\n      - name: nrf-volume\n        projected:\n
      \         sources:\n          - configMap:\n              name: nrf-configmap\n"
    nrf/nrf-service.yaml: |
      ---
      apiVersion: v1
      kind: Service
      metadata:
        name: nrf-nnrf
        labels:
          app.kubernetes.io/version: "v3.1.1"
          project: free5gc
          nf: nrf
      spec:
        type: ClusterIP
        ports:
          - port: 8000
            targetPort: 8000
            protocol: TCP
            name: http
        selector:
          project: free5gc
          nf: nrf
    nssf/nssf-configmap.yaml: "---\napiVersion: v1\nkind: ConfigMap\nmetadata:\n  name:
      nssf-configmap\n  labels:\n    app.kubernetes.io/version: \"v3.1.1\"\n    app:
      free5gc\ndata:\n  nssfcfg.yaml: |\n    info:\n      version: 1.0.1\n      description:
      NSSF initial local configuration\n\n    configuration:\n      serviceNameList:\n
      \       - nnssf-nsselection\n        - nnssf-nssaiavailability\n\n      sbi:\n
      \       scheme: http\n        registerIPv4: nssf-nnssf  # IP used to register
      to NRF\n        bindingIPv4: 0.0.0.0      # IP used to bind the service\n        port:
      80\n        tls:\n          key: config/TLS/nssf.key\n          pem: config/TLS/nssf.pem\n
      \     \n      nrfUri: http://nrf-nnrf:8000\n      \n      nsiList:\n        -
      snssai:\n            sst: 1\n          nsiInformationList:\n            - nrfId:
      http://nrf-nnrf:8000/nnrf-nfm/v1/nf-instances\n              nsiId: 10\n        -
      snssai:\n            sst: 1\n            sd: 1\n          nsiInformationList:\n
      \           - nrfId: http://nrf-nnrf:8000/nnrf-nfm/v1/nf-instances\n              nsiId:
      11\n        - snssai:\n            sst: 1\n            sd: 2\n          nsiInformationList:\n
      \           - nrfId: http://nrf-nnrf:8000/nnrf-nfm/v1/nf-instances\n              nsiId:
      12\n            - nrfId: http://nrf-nnrf:8000/nnrf-nfm/v1/nf-instances\n              nsiId:
      12\n        - snssai:\n            sst: 1\n            sd: 3\n          nsiInformationList:\n
      \           - nrfId: http://nrf-nnrf:8000/nnrf-nfm/v1/nf-instances\n              nsiId:
      13\n        - snssai:\n            sst: 2\n          nsiInformationList:\n            -
      nrfId: http://nrf-nnrf:8000/nnrf-nfm/v1/nf-instances\n              nsiId: 20\n
      \       - snssai:\n            sst: 2\n            sd: 1\n          nsiInformationList:\n
      \           - nrfId: http://nrf-nnrf:8000/nnrf-nfm/v1/nf-instances\n              nsiId:
      21\n        - snssai:\n            sst: 1\n            sd: 010203\n          nsiInformationList:\n
      \           - nrfId: http://nrf-nnrf:8000/nnrf-nfm/v1/nf-instances\n              nsiId:
      22\n      amfSetList:\n        - amfSetId: 1\n          amfList:\n            -
      ffa2e8d7-3275-49c7-8631-6af1df1d9d26\n            - 0e8831c3-6286-4689-ab27-1e2161e15cb1\n
      \           - a1fba9ba-2e39-4e22-9c74-f749da571d0d\n          nrfAmfSet: http://nrf-nnrf:8081/nnrf-nfm/v1/nf-instances\n
      \         supportedNssaiAvailabilityData:\n            - tai:\n                plmnId:\n
      \                 mcc: 466\n                  mnc: 92\n                tac:
      33456\n              supportedSnssaiList:\n                - sst: 1\n                  sd:
      1\n                - sst: 1\n                  sd: 2\n                - sst:
      2\n                  sd: 1\n            - tai:\n                plmnId:\n                  mcc:
      466\n                  mnc: 92\n                tac: 33457\n              supportedSnssaiList:\n
      \               - sst: 1\n                - sst: 1\n                  sd: 1\n
      \               - sst: 1\n                  sd: 2\n        - amfSetId: 2\n          nrfAmfSet:
      http://nrf-nnrf:8084/nnrf-nfm/v1/nf-instances\n          supportedNssaiAvailabilityData:\n
      \           - tai:\n                plmnId:\n                  mcc: 466\n                  mnc:
      92\n                tac: 33456\n              supportedSnssaiList:\n                -
      sst: 1\n                - sst: 1\n                  sd: 1\n                -
      sst: 1\n                  sd: 3\n                - sst: 2\n                  sd:
      1\n            - tai:\n                plmnId:\n                  mcc: 466\n
      \                 mnc: 92\n                tac: 33458\n              supportedSnssaiList:\n
      \               - sst: 1\n                - sst: 1\n                  sd: 1\n
      \               - sst: 2\n      nssfName: NSSF\n      supportedPlmnList:\n        -
      mcc: 208\n          mnc: 93\n      supportedNssaiInPlmnList:\n        - plmnId:\n
      \           mcc: 208\n            mnc: 93\n          supportedSnssaiList:\n
      \           - sst: 1\n              sd: 010203\n            - sst: 1\n              sd:
      112233\n            - sst: 1\n              sd: 3\n            - sst: 2\n              sd:
      1\n            - sst: 2\n              sd: 2\n      amfList:\n        - nfId:
      469de254-2fe5-4ca0-8381-af3f500af77c\n          supportedNssaiAvailabilityData:\n
      \           - tai:\n                plmnId:\n                  mcc: 466\n                  mnc:
      92\n                tac: 33456\n              supportedSnssaiList:\n                -
      sst: 1\n                - sst: 1\n                  sd: 2\n                -
      sst: 2\n            - tai:\n                plmnId:\n                  mcc:
      466\n                  mnc: 92\n                tac: 33457\n              supportedSnssaiList:\n
      \               - sst: 1\n                  sd: 1\n                - sst: 1\n
      \                 sd: 2\n        - nfId: fbe604a8-27b2-417e-bd7c-8a7be2691f8d\n
      \         supportedNssaiAvailabilityData:\n            - tai:\n                plmnId:\n
      \                 mcc: 466\n                  mnc: 92\n                tac:
      33458\n              supportedSnssaiList:\n                - sst: 1\n                -
      sst: 1\n                  sd: 1\n                - sst: 1\n                  sd:
      3\n                - sst: 2\n            - tai:\n                plmnId:\n                  mcc:
      466\n                  mnc: 92\n                tac: 33459\n              supportedSnssaiList:\n
      \               - sst: 1\n                - sst: 1\n                  sd: 1\n
      \               - sst: 2\n                - sst: 2\n                  sd: 1\n
      \       - nfId: b9e6e2cb-5ce8-4cb6-9173-a266dd9a2f0c\n          supportedNssaiAvailabilityData:\n
      \           - tai:\n                plmnId:\n                  mcc: 466\n                  mnc:
      92\n                tac: 33456\n              supportedSnssaiList:\n                -
      sst: 1\n                - sst: 1\n                  sd: 1\n                -
      sst: 1\n                  sd: 2\n                - sst: 2\n            - tai:\n
      \               plmnId:\n                  mcc: 466\n                  mnc:
      92\n                tac: 33458\n              supportedSnssaiList:\n                -
      sst: 1\n                - sst: 1\n                  sd: 1\n                -
      sst: 2\n                - sst: 2\n                  sd: 1\n      taList:\n        -
      tai:\n            plmnId:\n              mcc: 466\n              mnc: 92\n            tac:
      33456\n          accessType: 3GPP_ACCESS\n          supportedSnssaiList:\n            -
      sst: 1\n            - sst: 1\n              sd: 1\n            - sst: 1\n              sd:
      2\n            - sst: 2\n        - tai:\n            plmnId:\n              mcc:
      466\n              mnc: 92\n            tac: 33457\n          accessType: 3GPP_ACCESS\n
      \         supportedSnssaiList:\n            - sst: 1\n            - sst: 1\n
      \             sd: 1\n            - sst: 1\n              sd: 2\n            -
      sst: 2\n        - tai:\n            plmnId:\n              mcc: 466\n              mnc:
      92\n            tac: 33458\n          accessType: 3GPP_ACCESS\n          supportedSnssaiList:\n
      \           - sst: 1\n            - sst: 1\n              sd: 1\n            -
      sst: 1\n              sd: 3\n            - sst: 2\n          restrictedSnssaiList:\n
      \           - homePlmnId:\n                mcc: 310\n                mnc: 560\n
      \             sNssaiList:\n                - sst: 1\n                  sd: 3\n
      \       - tai:\n            plmnId:\n              mcc: 466\n              mnc:
      92\n            tac: 33459\n          accessType: 3GPP_ACCESS\n          supportedSnssaiList:\n
      \           - sst: 1\n            - sst: 1\n              sd: 1\n            -
      sst: 2\n            - sst: 2\n              sd: 1\n          restrictedSnssaiList:\n
      \           - homePlmnId:\n                mcc: 310\n                mnc: 560\n
      \             sNssaiList:\n                - sst: 2\n                  sd: 1\n
      \     mappingListFromPlmn:\n        - operatorName: NTT Docomo\n          homePlmnId:\n
      \           mcc: 440\n            mnc: 10\n          mappingOfSnssai:\n            -
      servingSnssai:\n                sst: 1\n                sd: 1\n              homeSnssai:\n
      \               sst: 1\n                sd: 1\n            - servingSnssai:\n
      \               sst: 1\n                sd: 2\n              homeSnssai:\n                sst:
      1\n                sd: 3\n            - servingSnssai:\n                sst:
      1\n                sd: 3\n              homeSnssai:\n                sst: 1\n
      \               sd: 4\n            - servingSnssai:\n                sst: 2\n
      \               sd: 1\n              homeSnssai:\n                sst: 2\n                sd:
      2\n        - operatorName: AT&T Mobility\n          homePlmnId:\n            mcc:
      310\n            mnc: 560\n          mappingOfSnssai:\n            - servingSnssai:\n
      \               sst: 1\n                sd: 1\n              homeSnssai:\n                sst:
      1\n                sd: 2\n            - servingSnssai:\n                sst:
      1\n                sd: 2\n              homeSnssai:\n                sst: 1\n
      \               sd: 3      \n\n    logger:\n      NSSF:\n        ReportCaller:
      false\n        debugLevel: info\n"
    nssf/nssf-deployment.yaml: "---\napiVersion: apps/v1\nkind: Deployment\nmetadata:\n
      \ name: free5gc-nssf\n  labels:\n    app.kubernetes.io/version: \"v3.1.1\"\n
      \   project: free5gc\n    nf: nssf\nspec:\n  replicas: 1\n  selector:\n    matchLabels:\n
      \     project: free5gc\n      nf: nssf\n  template:\n    metadata:\n      labels:\n
      \       project: free5gc\n        nf: nssf\n    spec:\n      initContainers:\n
      \     - name: wait-nrf\n        image: towards5gs/initcurl:1.0.0\n        env:\n
      \       - name: DEPENDENCIES\n          value: http://nrf-nnrf:8000\n        command:
      ['sh', '-c', 'set -x; for dependency in $DEPENDENCIES; do while [ $(curl --insecure
      --connect-timeout 1 -s -o /dev/null -w \"%{http_code}\" $dependency) -ne 200
      ]; do echo waiting for dependencies; sleep 1; done; done;']\n\n      containers:\n
      \     - name: nssf\n        image: towards5gs/free5gc-nssf:v3.1.1\n        imagePullPolicy:
      IfNotPresent\n        securityContext:\n            {}\n        ports:\n        -
      containerPort: 80\n        command: [\"./nssf\"]\n        args: [\"-c\", \"../config/nssfcfg.yaml\"]\n
      \       env: \n          - name: GIN_MODE\n            value: release\n        volumeMounts:\n
      \       - mountPath: /free5gc/config/\n          name: nssf-volume\n        resources:\n
      \           limits:\n              cpu: 100m\n              memory: 128Mi\n
      \           requests:\n              cpu: 100m\n              memory: 128Mi\n
      \     dnsPolicy: ClusterFirst\n      restartPolicy: Always\n\n      volumes:\n
      \     - name: nssf-volume\n        projected:\n          sources:\n          -
      configMap:\n              name: nssf-configmap\n"
    nssf/nssf-service.yaml: |
      ---
      apiVersion: v1
      kind: Service
      metadata:
        name: nssf-nnssf
        labels:
          app.kubernetes.io/version: "v3.1.1"
          project: free5gc
          nf: nssf
      spec:
        type: ClusterIP
        ports:
          - port: 80
            targetPort: 80
            protocol: TCP
            name: http
        selector:
          project: free5gc
          nf: nssf
    package-context.yaml: |
      apiVersion: v1
      kind: ConfigMap
      metadata:
        name: kptfile.kpt.dev
        annotations:
          config.kubernetes.io/local-config: "true"
      data:
        name: free5gc
        namespace: free5gc
    pcf/pcf-configmap.yaml: "---\napiVersion: v1\nkind: ConfigMap\nmetadata:\n  name:
      pcf-configmap\n  labels:\n    app.kubernetes.io/version: \"v3.1.1\"\n    app:
      free5gc\ndata:\n  pcfcfg.yaml: |\n    info:\n      version: 1.0.1\n      description:
      PCF initial local configuration\n\n    configuration:\n      serviceList:\n
      \       - serviceName: npcf-am-policy-control\n        - serviceName: npcf-smpolicycontrol\n
      \         suppFeat: 3fff\n        - serviceName: npcf-bdtpolicycontrol\n        -
      serviceName: npcf-policyauthorization\n          suppFeat: 3\n        - serviceName:
      npcf-eventexposure\n        - serviceName: npcf-ue-policy-control\n\n      sbi:\n
      \       scheme: http\n        registerIPv4: pcf-npcf  # IP used to register
      to NRF\n        bindingIPv4: 0.0.0.0    # IP used to bind the service\n        port:
      80\n        tls:\n          key: config/TLS/pcf.key\n          pem: config/TLS/pcf.pem\n
      \     \n      mongodb:       # the mongodb connected by this PCF\n        name:
      free5gc                  # name of the mongodb\n        url: mongodb://mongodb:27017
      # a valid URL of the mongodb\n      \n      nrfUri: http://nrf-nnrf:8000\n      pcfName:
      PCF\n      timeFormat: 2019-01-02 15:04:05\n      defaultBdtRefId: BdtPolicyId-\n
      \     locality: area1\n\n    logger:\n      PCF:\n        ReportCaller: false\n
      \       debugLevel: info\n"
    pcf/pcf-deployment.yaml: |
      ---
      apiVersion: apps/v1
      kind: Deployment
      metadata:
        name: free5gc-pcf
        labels:
          app.kubernetes.io/version: "v3.1.1"
          project: free5gc
          nf: pcf
      spec:
        replicas: 1
        selector:
          matchLabels:
            project: free5gc
            nf: pcf
        template:
          metadata:
            labels:
              project: free5gc
              nf: pcf
          spec:
            initContainers:
            - name: wait-nrf
              image: towards5gs/initcurl:1.0.0
              env:
              - name: DEPENDENCIES
                value: http://nrf-nnrf:8000
              command: ['sh', '-c', 'set -x; for dependency in $DEPENDENCIES; do while [ $(curl --insecure --connect-timeout 1 -s -o /dev/null -w "%{http_code}" $dependency) -ne 200 ]; do echo waiting for dependencies; sleep 1; done; done;']

            containers:
            - name: pcf
              image: towards5gs/free5gc-pcf:v3.1.1
              imagePullPolicy: IfNotPresent
              ports:
              - containerPort: 80
              command: ["./pcf"]
              args: ["-c", "../config/pcfcfg.yaml"]
              env:
                - name: GIN_MODE
                  value: release
              volumeMounts:
              - mountPath: /free5gc/config/
                name: pcf-volume
              resources:
                  limits:
                    cpu: 100m
                    memory: 128Mi
                  requests:
                    cpu: 100m
                    memory: 128Mi
            dnsPolicy: ClusterFirst
            restartPolicy: Always

            volumes:
            - name: pcf-volume
              projected:
                sources:
                - configMap:
                    name: pcf-configmap
    pcf/pcf-service.yaml: |
      ---
      apiVersion: v1
      kind: Service
      metadata:
        name: pcf-npcf
        labels:
          app.kubernetes.io/version: "v3.1.1"
          project: free5gc
          nf: pcf
      spec:
        type: ClusterIP
        ports:
          - port: 80
            targetPort: 80
            protocol: TCP
            name: http
        selector:
          project: free5gc
          nf: pcf
    udm/udm-configmap.yaml: "---\napiVersion: v1\nkind: ConfigMap\nmetadata:\n  name:
      udm-configmap\n  labels:\n    app.kubernetes.io/version: \"v3.1.1\"\n    app:
      free5gc\ndata:\n  udmcfg.yaml: |\n    info:\n      version: 1.0.2\n      description:
      UDM initial local configuration\n\n    configuration:\n      serviceNameList:\n
      \       - nudm-sdm\n        - nudm-uecm\n        - nudm-ueau\n        - nudm-ee\n
      \       - nudm-pp\n      \n      sbi:\n        scheme: http\n        registerIPv4:
      udm-nudm # IP used to register to NRF\n        bindingIPv4: 0.0.0.0  # IP used
      to bind the service\n        port: 80\n        tls:\n          key: config/TLS/udm.key\n
      \         pem: config/TLS/udm.pem\n      \n      nrfUri: http://nrf-nnrf:8000\n
      \     # test data set from TS33501-f60 Annex C.4\n      SuciProfile:\n        -
      ProtectionScheme: 1 # Protect Scheme: Profile A\n          PrivateKey: c53c22208b61860b06c62e5406a7b330c2b577aa5558981510d128247d38bd1d\n
      \         PublicKey: 5a8d38864820197c3394b92613b20b91633cbd897119273bf8e4a6f4eec0a650\n
      \       - ProtectionScheme: 2 # Protect Scheme: Profile B\n          PrivateKey:
      F1AB1074477EBCC7F554EA1C5FC368B1616730155E0041AC447D6301975FECDA\n          PublicKey:
      0472DA71976234CE833A6907425867B82E074D44EF907DFB4B3E21C1C2256EBCD15A7DED52FCBB097A4ED250E036C7B9C8C7004C4EEDC4F068CD7BF8D3F900E3B4\n\n
      \   logger:\n      UDM:\n        ReportCaller: false\n        debugLevel: info\n"
    udm/udm-deployment.yaml: "---\napiVersion: apps/v1\nkind: Deployment\nmetadata:\n
      \ name: free5gc-udm\n  labels:\n    app.kubernetes.io/version: \"v3.1.1\"\n
      \   project: free5gc\n    nf: udm\nspec:\n  replicas: 1\n  selector:\n    matchLabels:\n
      \     project: free5gc\n      nf: udm\n  template:\n    metadata:\n      labels:\n
      \       project: free5gc\n        nf: udm\n    spec:\n      initContainers:\n
      \     - name: wait-nrf\n        image: towards5gs/initcurl:1.0.0\n        env:\n
      \       - name: DEPENDENCIES\n          value: http://nrf-nnrf:8000\n        command:
      ['sh', '-c', 'set -x; for dependency in $DEPENDENCIES; do while [ $(curl --insecure
      --connect-timeout 1 -s -o /dev/null -w \"%{http_code}\" $dependency) -ne 200
      ]; do echo waiting for dependencies; sleep 1; done; done;']\n\n      containers:\n
      \     - name: udm\n        image: towards5gs/free5gc-udm:v3.1.1\n        imagePullPolicy:
      IfNotPresent\n        ports:\n        - containerPort: 80\n        command:
      [\"./udm\"]\n        args: [\"-c\", \"../config/udmcfg.yaml\"]\n        env:
      \n          - name: GIN_MODE\n            value: release\n        volumeMounts:\n
      \       - mountPath: /free5gc/config/\n          name: udm-volume\n        resources:\n
      \           limits:\n              cpu: 100m\n              memory: 128Mi\n
      \           requests:\n              cpu: 100m\n              memory: 128Mi\n
      \     dnsPolicy: ClusterFirst\n      restartPolicy: Always\n\n      volumes:\n
      \     - name: udm-volume\n        projected:\n          sources:\n          -
      configMap:\n              name: udm-configmap\n"
    udm/udm-service.yaml: |
      ---
      apiVersion: v1
      kind: Service
      metadata:
        name: udm-nudm
        labels:
          app.kubernetes.io/version: "v3.1.1"
          project: free5gc
          nf: udm
      spec:
        type: ClusterIP
        ports:
          - port: 80
            targetPort: 80
            protocol: TCP
            name: http
        selector:
          project: free5gc
          nf: udm
    udr/udr-configmap.yaml: "---\napiVersion: v1\nkind: ConfigMap\nmetadata:\n  name:
      udr-configmap\n  labels:\n    app.kubernetes.io/version: \"v3.1.1\"\n    app:
      free5gc\ndata:\n  udrcfg.yaml: |\n    info:\n      version: 1.0.1\n      description:
      UDR initial local configuration\n\n    configuration:\n      sbi:\n        scheme:
      http\n        registerIPv4: udr-nudr # IP used to register to NRF\n        bindingIPv4:
      0.0.0.0  # IP used to bind the service\n        port: 80\n        tls:\n          key:
      config/TLS/udr.key\n          pem: config/TLS/udr.pem\n\n      mongodb:\n        name:
      free5gc\n        url: mongodb://mongodb:27017       \n      \n      nrfUri:
      http://nrf-nnrf:8000\n\n    logger:\n      MongoDBLibrary:\n        ReportCaller:
      false\n        debugLevel: info\n      OpenApi:\n        ReportCaller: false\n
      \       debugLevel: info\n      PathUtil:\n        ReportCaller: false\n        debugLevel:
      info\n      UDR:\n        ReportCaller: false\n        debugLevel: info\n"
    udr/udr-deployment.yaml: "---\napiVersion: apps/v1\nkind: Deployment\nmetadata:\n
      \ name: free5gc-udr\n  labels:\n    app.kubernetes.io/version: \"v3.1.1\"\n
      \   project: free5gc\n    nf: udr\nspec:\n  replicas: 1\n  selector:\n    matchLabels:\n
      \     project: free5gc\n      nf: udr\n  template:\n    metadata:\n      labels:\n
      \       project: free5gc\n        nf: udr\n    spec:\n      initContainers:\n
      \     - name: wait-nrf\n        image: towards5gs/initcurl:1.0.0\n        env:\n
      \       - name: DEPENDENCIES\n          value: http://nrf-nnrf:8000\n        command:
      ['sh', '-c', 'set -x; for dependency in $DEPENDENCIES; do while [ $(curl --insecure
      --connect-timeout 1 -s -o /dev/null -w \"%{http_code}\" $dependency) -ne 200
      ]; do echo waiting for dependencies; sleep 1; done; done;']\n\n      containers:\n
      \     - name: udr\n        image: towards5gs/free5gc-udr:v3.1.1\n        imagePullPolicy:
      IfNotPresent\n        ports:\n        - containerPort: 80\n        command:
      [\"./udr\"]\n        args: [\"-c\", \"../config/udrcfg.yaml\"]\n        env:
      \n          - name: DB_URI\n            value: mongodb://mongodb/free5gc\n          -
      name: GIN_MODE\n            value: release\n        volumeMounts:\n        -
      mountPath: /free5gc/config/\n          name: udr-volume\n        resources:\n
      \           limits:\n              cpu: 100m\n              memory: 128Mi\n
      \           requests:\n              cpu: 100m\n              memory: 128Mi\n
      \     dnsPolicy: ClusterFirst\n      restartPolicy: Always\n\n      volumes:\n
      \     - name: udr-volume\n        projected:\n          sources:\n          -
      configMap:\n              name: udr-configmap\n"
    udr/udr-service.yaml: |
      ---
      apiVersion: v1
      kind: Service
      metadata:
        name: udr-nudr
        labels:
          app.kubernetes.io/version: "v3.1.1"
          project: free5gc
          nf: udr
      spec:
        type: ClusterIP
        ports:
          - port: 80
            targetPort: 80
            protocol: TCP
            name: http
        selector:
          project: free5gc
          nf: udr
    webui/webui-configmap.yaml: "---\napiVersion: v1\nkind: ConfigMap\nmetadata:\n
      \ name: webui-configmap\n  labels:\n    app.kubernetes.io/version: \"v3.1.1\"\n
      \   app: free5gc\ndata:\n  webuicfg.yaml: |\n    info:\n      version: 1.0.0\n
      \     description: WEBUI initial local configuration\n\n    configuration:\n
      \     mongodb:\n        name: free5gc\n        url: mongodb://mongodb:27017\n
      \       \n    logger:\n      WEBUI:\n        ReportCaller: false\n        debugLevel:
      info\n"
    webui/webui-deployment.yaml: |
      ---
      apiVersion: apps/v1
      kind: Deployment
      metadata:
        name: free5gc-webui
        labels:
          app.kubernetes.io/version: "v3.1.1"
          project: free5gc
          nf: webui
      spec:
        replicas: 1
        selector:
          matchLabels:
            project: free5gc
            nf: webui
        template:
          metadata:
            labels:
              project: free5gc
              nf: webui
          spec:
            initContainers:
            - name: wait-mongo
              image: busybox:1.32.0
              env:
              - name: DEPENDENCIES
                value: mongodb:27017
              command: ["sh", "-c", "until nc -z $DEPENDENCIES; do echo waiting for the MongoDB; sleep 2; done;"]
            containers:
            - name: webui
              image: towards5gs/free5gc-webui:v3.1.1
              imagePullPolicy: IfNotPresent
              ports:
              - containerPort: 5000
              command: ["./webconsole"]
              args: ["-c", "../config/webuicfg.yaml"]
              env:
                - name: GIN_MODE
                  value: release
              volumeMounts:
              - mountPath: /free5gc/config/
                name: webui-volume
              resources:
                  limits:
                    cpu: 100m
                    memory: 128Mi
                  requests:
                    cpu: 100m
                    memory: 128Mi
              readinessProbe:
                initialDelaySeconds: 0
                periodSeconds: 1
                timeoutSeconds: 1
                failureThreshold:  40
                successThreshold: 1
                httpGet:
                  scheme: HTTP
                  port: 5000
              livenessProbe:
                initialDelaySeconds: 120
                periodSeconds: 10
                timeoutSeconds: 10
                failureThreshold: 3
                successThreshold: 1
                httpGet:
                  scheme: HTTP
                  port: 5000
            dnsPolicy: ClusterFirst
            restartPolicy: Always

            volumes:
            - name: webui-volume
              projected:
                sources:
                - configMap:
                    name: webui-configmap
    webui/webui-service.yaml: |
      ---
      apiVersion: v1
      kind: Service
      metadata:
        name: webui-service
        labels:
          app.kubernetes.io/version: "v3.1.1"
          project: free5gc
          nf: webui
      spec:
        type: NodePort
        ports:
          - port: 5000
            targetPort: 5000
            nodePort: 30500
            protocol: TCP
            name: http
        selector:
          project: free5gc
          nf: webui
  revision: v1
  workspaceName: v1
status:
  renderStatus:
    error: ""
    result:
      exitCode: 0
      metadata:
        creationTimestamp: null
```
</details>

## The porchctl command

The `porchtcl` command is an administration command for acting on Porch `Repository` (repo) and `PackageRevision` (rpkg)
CRs. See its [documentation for usage information]({{% relref "/docs/7_cli_api/relevant_old_docs/porchctl-cli-guide.md" %}}).

Check that <code>porchctl</code> lists our repositories:</summary>

```bash
porchctl repo -n porch-demo get
NAME                  TYPE   CONTENT   DEPLOYMENT   READY   ADDRESS
edge1                 git    Package   true         True    http://172.18.255.200:3000/nephio/edge1.git
external-blueprints   git    Package   false        True    https://github.com/nephio-project/free5gc-packages.git
management            git    Package   false        True    http://172.18.255.200:3000/nephio/management.git
```

<details>
<summary>Check that porchctl lists our remote packages (PackageRevisions):</summary>

```
porchctl rpkg -n porch-demo get
NAME                                                           PACKAGE              WORKSPACENAME   REVISION   LATEST   LIFECYCLE   REPOSITORY
porch-test.network-function.great-outdoors                     free5gc-cp           main            main       false    Published   external-blueprints
porch-test.network-function.great-outdoors                     free5gc-cp           v1              v1         true     Published   external-blueprints
porch-test.network-function.great-outdoors                     free5gc-operator     main            main       false    Published   external-blueprints
porch-test.network-function.great-outdoors                     free5gc-operator     v1              v1         false    Published   external-blueprints
porch-test.network-function.great-outdoors                     free5gc-operator     v2              v2         false    Published   external-blueprints
porch-test.network-function.great-outdoors                     free5gc-operator     v3              v3         false    Published   external-blueprints
porch-test.network-function.great-outdoors                     free5gc-operator     v4              v4         false    Published   external-blueprints
porch-test.network-function.great-outdoors                     free5gc-operator     v5              v5         true     Published   external-blueprints
porch-test.network-function.great-outdoors                     free5gc-upf          main            main       false    Published   external-blueprints
porch-test.network-function.great-outdoors                     free5gc-upf          v1              v1         true     Published   external-blueprints
porch-test.network-function.great-outdoors                     pkg-example-amf-bp   main            main       false    Published   external-blueprints
porch-test.network-function.great-outdoors                     pkg-example-amf-bp   v1              v1         false    Published   external-blueprints
porch-test.network-function.great-outdoors                     pkg-example-amf-bp   v2              v2         false    Published   external-blueprints
porch-test.network-function.great-outdoors                     pkg-example-amf-bp   v3              v3         false    Published   external-blueprints
porch-test.network-function.great-outdoors                     pkg-example-amf-bp   v4              v4         false    Published   external-blueprints
porch-test.network-function.great-outdoors                     pkg-example-amf-bp   v5              v5         true     Published   external-blueprints
porch-test.network-function.great-outdoors                     pkg-example-smf-bp   main            main       false    Published   external-blueprints
porch-test.network-function.great-outdoors                     pkg-example-smf-bp   v1              v1         false    Published   external-blueprints
porch-test.network-function.great-outdoors                     pkg-example-smf-bp   v2              v2         false    Published   external-blueprints
porch-test.network-function.great-outdoors                     pkg-example-smf-bp   v3              v3         false    Published   external-blueprints
porch-test.network-function.great-outdoors                     pkg-example-smf-bp   v4              v4         false    Published   external-blueprints
porch-test.network-function.great-outdoors                     pkg-example-smf-bp   v5              v5         true     Published   external-blueprints
porch-test.network-function.great-outdoors                     pkg-example-upf-bp   main            main       false    Published   external-blueprints
porch-test.network-function.great-outdoors                     pkg-example-upf-bp   v1              v1         false    Published   external-blueprints
porch-test.network-function.great-outdoors                     pkg-example-upf-bp   v2              v2         false    Published   external-blueprints
porch-test.network-function.great-outdoors                     pkg-example-upf-bp   v3              v3         false    Published   external-blueprints
porch-test.network-function.great-outdoors                     pkg-example-upf-bp   v4              v4         false    Published   external-blueprints
porch-test.network-function.great-outdoors                     pkg-example-upf-bp   v5              v5         true     Published   external-blueprints
```
</details>

The output above is similar to the output of `kubectl get packagerevision -n porch-demo` above.

## Creating a blueprint in Porch

### Blueprint with no Kpt pipelines

Create a new package in our *management* repository using the sample *network-function* package provided. This network
function Kpt package is a demo Kpt package that installs [Nginx](https://github.com/nginx). 

```
porchctl -n porch-demo rpkg init network-function --repository=management --workspace=v1
management-8b80738a6e0707e3718ae1db3668d0b8ca3f1c82 created
porchctl -n porch-demo rpkg get --name network-function
NAME                                                  PACKAGE            WORKSPACENAME   REVISION   LATEST   LIFECYCLE   REPOSITORY
management-8b80738a6e0707e3718ae1db3668d0b8ca3f1c82   network-function   v1                         false    Draft       management
```

This command creates a new *PackageRevision* CR in porch and also creates a branch called *network-function/v1* in our
Gitea *management* repository. Use the Gitea web UI to confirm that the branch has been created and note that it only has
default content as yet.

We now pull the package we have initialized from Porch.

``` 
porchctl -n porch-demo rpkg pull management-8b80738a6e0707e3718ae1db3668d0b8ca3f1c82 blueprints/initialized/network-function
```

We update the initialized package and add our local changes.
```
cp blueprints/local-changes/network-function/* blueprints/initialized/network-function 
```

Now, we push the package contents to porch:
```
porchctl -n porch-demo rpkg push management-8b80738a6e0707e3718ae1db3668d0b8ca3f1c82 blueprints/initialized/network-function
```

Check on the Gitea web UI and we can see that the actual package contents have been pushed.

Now we propose and approve the package.

```
porchctl -n porch-demo rpkg propose management-8b80738a6e0707e3718ae1db3668d0b8ca3f1c82
management-8b80738a6e0707e3718ae1db3668d0b8ca3f1c82 proposed

porchctl -n porch-demo rpkg get --name network-function                                
NAME                                                  PACKAGE            WORKSPACENAME   REVISION   LATEST   LIFECYCLE   REPOSITORY
management-8b80738a6e0707e3718ae1db3668d0b8ca3f1c82   network-function   v1                         false    Proposed    management

porchctl -n porch-demo rpkg approve management-8b80738a6e0707e3718ae1db3668d0b8ca3f1c82
management-8b80738a6e0707e3718ae1db3668d0b8ca3f1c82 approved

porchctl -n porch-demo rpkg get --name network-function                                
NAME                                                  PACKAGE            WORKSPACENAME   REVISION   LATEST   LIFECYCLE   REPOSITORY
management-8b80738a6e0707e3718ae1db3668d0b8ca3f1c82   network-function   v1              v1         true     Published   management

```

Once we approve the package, the package is merged into the main branch in the *management* repository and the branch called
*network-function/v1* in that repository is removed. Use the Gitea UI to verify this. We now have our blueprint package in our
*management* repository and we can deploy this package into workload clusters.

### Blueprint with a Kpt pipeline

The second blueprint in the *blueprint* directory is called *network-function-auto-namespace*. This network
function is exactly the same as the *network-function* package except that it has a Kpt function that automatically
creates a namespace with the namespace configured in the name field in the *package-context.yaml* file. Note that no
namespace is defined in the metadata of the *deployment.yaml* file of this Kpt package.

We use the same sequence of commands again to publish our blueprint package for *network-function-auto-namespace*.

```
porchctl -n porch-demo rpkg init network-function-auto-namespace --repository=management --workspace=v1
management-c97bc433db93f2e8a3d413bed57216c2a72fc7e3 created

porchctl -n porch-demo rpkg pull management-c97bc433db93f2e8a3d413bed57216c2a72fc7e3 blueprints/initialized/network-function-auto-namespace

cp blueprints/local-changes/network-function-auto-namespace/* blueprints/initialized/network-function-auto-namespace

porchctl -n porch-demo rpkg push management-c97bc433db93f2e8a3d413bed57216c2a72fc7e3 blueprints/initialized/network-function-auto-namespace
```

Examine the *drafts/network-function-auto-namespace/v1* branch in Gitea. Notice that the set-namespace Kpt function in
the pipeline in the *Kptfile* has set the namespace in the *deployment.yaml* file to the value default-namespace-name,
which it read from the *package-context.yaml* file.

Now we propose and approve the package.

```
porchctl -n porch-demo rpkg propose management-c97bc433db93f2e8a3d413bed57216c2a72fc7e3
management-c97bc433db93f2e8a3d413bed57216c2a72fc7e3 proposed

porchctl -n porch-demo rpkg approve management-c97bc433db93f2e8a3d413bed57216c2a72fc7e3
management-c97bc433db93f2e8a3d413bed57216c2a72fc7e3 approved

porchctl -n porch-demo rpkg get --name network-function-auto-namespace
NAME                                                  PACKAGE                           WORKSPACENAME   REVISION   LATEST   LIFECYCLE   REPOSITORY
management-f9a6f2802111b9e81c296422c03aae279725f6df   network-function-auto-namespace   v1              main       false    Published   management
management-c97bc433db93f2e8a3d413bed57216c2a72fc7e3   network-function-auto-namespace   v1              v1         true     Published   management

```

## Deploying a blueprint onto a workload cluster

### Blueprint with no Kpt pipelines

The process of deploying a blueprint package from our *management* repository clones the package, then modifies it for use on
the workload cluster. The cloned package is then initialized, pushed, proposed, and approved onto the *edge1* repository.
Remember that the *edge1* repository is being monitored by configsync from the edge1 cluster, so once the package appears in
the *edge1* repository on the management cluster, it will be pulled by configsync and applied to the edge1 cluster.

```
porchctl -n porch-demo rpkg pull management-8b80738a6e0707e3718ae1db3668d0b8ca3f1c82 tmp_packages_for_deployment/edge1-network-function-a.clone.tmp

find tmp_packages_for_deployment/edge1-network-function-a.clone.tmp

tmp_packages_for_deployment/edge1-network-function-a.clone.tmp
tmp_packages_for_deployment/edge1-network-function-a.clone.tmp/deployment.yaml
tmp_packages_for_deployment/edge1-network-function-a.clone.tmp/.KptRevisionMetadata
tmp_packages_for_deployment/edge1-network-function-a.clone.tmp/README.md
tmp_packages_for_deployment/edge1-network-function-a.clone.tmp/Kptfile
tmp_packages_for_deployment/edge1-network-function-a.clone.tmp/package-context.yaml
```
The package we created in the last section is cloned. We now remove the original metadata from the package.
```
rm tmp_packages_for_deployment/edge1-network-function-a.clone.tmp/.KptRevisionMetadata
```

We use a *kpt* function to change the namespace that will be used for the deployment of the network function.

```
kpt fn eval --image=gcr.io/kpt-fn/set-namespace:v0.4.1 tmp_packages_for_deployment/edge1-network-function-a.clone.tmp -- namespace=edge1-network-function-a 

[RUNNING] "gcr.io/kpt-fn/set-namespace:v0.4.1"
[PASS] "gcr.io/kpt-fn/set-namespace:v0.4.1" in 300ms
  Results:
    [info]: namespace "" updated to "edge1-network-function-a", 1 value(s) changed
```

We now initialize and push the package to the *edge1* repository:

```
porchctl -n porch-demo rpkg init edge1-network-function-a --repository=edge1 --workspace=v1
edge1-d701be9b849b8b8724a6e052cbc74ca127b737c3 created

porchctl -n porch-demo rpkg pull edge1-d701be9b849b8b8724a6e052cbc74ca127b737c3 tmp_packages_for_deployment/edge1-network-function-a

cp tmp_packages_for_deployment/edge1-network-function-a.clone.tmp/* tmp_packages_for_deployment/edge1-network-function-a
rm -fr tmp_packages_for_deployment/edge1-network-function-a.clone.tmp

porchctl -n porch-demo rpkg push edge1-d701be9b849b8b8724a6e052cbc74ca127b737c3 tmp_packages_for_deployment/edge1-network-function-a

porchctl -n porch-demo rpkg get --name edge1-network-function-a
NAME                                             PACKAGE              WORKSPACENAME   REVISION   LATEST   LIFECYCLE   REPOSITORY
edge1-d701be9b849b8b8724a6e052cbc74ca127b737c3   network-function-a   v1                         false    Draft       edge1
```

You can verify that the package is in the *network-function-a/v1* branch of the deployment repository using the Gitea web UI.


Check that the *edge1-network-function-a* package is not deployed on the edge1 cluster yet:
```
export KUBECONFIG=~/.kube/kind-edge1-config

kubectl get pod -n edge1-network-function-a
No resources found in network-function-a namespace.

```

We now propose and approve the deployment package, which merges the package to the *edge1* repository and further triggers
configsync to apply the package to the edge1 cluster.

```
export KUBECONFIG=~/.kube/kind-management-config

porchctl -n porch-demo rpkg propose edge1-d701be9b849b8b8724a6e052cbc74ca127b737c3
edge1-d701be9b849b8b8724a6e052cbc74ca127b737c3 proposed

porchctl -n porch-demo rpkg approve edge1-d701be9b849b8b8724a6e052cbc74ca127b737c3
edge1-d701be9b849b8b8724a6e052cbc74ca127b737c3 approved

porchctl -n porch-demo rpkg get --name edge1-network-function-a
NAME                                             PACKAGE              WORKSPACENAME   REVISION   LATEST   LIFECYCLE   REPOSITORY
edge1-d701be9b849b8b8724a6e052cbc74ca127b737c3   network-function-a   v1              v1         true     Published   edge1
```

We can now check that the *network-function-a* package is deployed on the edge1 cluster and that the pod is running
```
export KUBECONFIG=~/.kube/kind-edge1-config

kubectl get pod -n edge1-network-function-a
No resources found in network-function-a namespace.

kubectl get pod -n edge1-network-function-a
NAME                               READY   STATUS              RESTARTS   AGE
network-function-9779fc9f5-4rqp2   1/1     ContainerCreating   0          9s

kubectl get pod -n edge1-network-function-a
NAME                               READY   STATUS    RESTARTS   AGE
network-function-9779fc9f5-4rqp2   1/1     Running   0          44s
```

### Blueprint with a Kpt pipeline

The process for deploying a blueprint with a *Kpt* pipeline runs the Kpt pipeline automatically with whatever configuration we give it. Rather than explicitly running a *Kpt* function to change the namespace, we will specify the namespace as configuration and the pipeline will apply it to the deployment.

```
porchctl -n porch-demo rpkg pull management-c97bc433db93f2e8a3d413bed57216c2a72fc7e3 tmp_packages_for_deployment/edge1-network-function-auto-namespace-a.clone.tmp

find tmp_packages_for_deployment/edge1-network-function-auto-namespace-a.clone.tmp

tmp_packages_for_deployment/edge1-network-function-auto-namespace-a.clone.tmp
tmp_packages_for_deployment/edge1-network-function-auto-namespace-a.clone.tmp/deployment.yaml
tmp_packages_for_deployment/edge1-network-function-auto-namespace-a.clone.tmp/.KptRevisionMetadata
tmp_packages_for_deployment/edge1-network-function-auto-namespace-a.clone.tmp/README.md
tmp_packages_for_deployment/edge1-network-function-auto-namespace-a.clone.tmp/Kptfile
tmp_packages_for_deployment/edge1-network-function-auto-namespace-a.clone.tmp/package-context.yaml
```

We now remove the original metadata from the package.
```
rm tmp_packages_for_deployment/edge1-network-function-auto-namespace-a.clone.tmp/.KptRevisionMetadata
```

The package we created in the last section is cloned. We now initialize and push the package to the *edge1* repository:

```
porchctl -n porch-demo rpkg init edge1-network-function-auto-namespace-a --repository=edge1 --workspace=v1
edge1-48997da49ca0a733b0834c1a27943f1a0e075180 created

porchctl -n porch-demo rpkg pull edge1-48997da49ca0a733b0834c1a27943f1a0e075180 tmp_packages_for_deployment/edge1-network-function-auto-namespace-a

cp tmp_packages_for_deployment/edge1-network-function-auto-namespace-a.clone.tmp/* tmp_packages_for_deployment/edge1-network-function-auto-namespace-a
rm -fr tmp_packages_for_deployment/edge1-network-function-auto-namespace-a.clone.tmp
```


We now simply configure the namespace we want to apply. edit the *tmp_packages_for_deployment/edge1-network-function-auto-namespace-a/package-context.yaml* file and set the namespace to use:

```
8c8
<   name: default-namespace-name
---
>   name: edge1-network-function-auto-namespace-a
```

We now push the package to the *edge1* repository:

```
porchctl -n porch-demo rpkg push edge1-48997da49ca0a733b0834c1a27943f1a0e075180 tmp_packages_for_deployment/edge1-network-function-auto-namespace-a
[RUNNING] "gcr.io/kpt-fn/set-namespace:v0.4.1" 
[PASS] "gcr.io/kpt-fn/set-namespace:v0.4.1"
  Results:
    [info]: namespace "default-namespace-name" updated to "edge1-network-function-auto-namespace-a", 1 value(s) changed

porchctl -n porch-demo rpkg get --name edge1-network-function-auto-namespace-a
```

You can verify that the package is in the *network-function-auto-namespace-a/v1* branch of the deployment repository using the
Gitea web UI. You can see that the kpt pipeline fired and set the edge1-network-function-auto-namespace-a namespace in
the *deployment.yaml* file on the *drafts/edge1-network-function-auto-namespace-a/v1* branch on the *edge1* repository in
Gitea.

Check that the *edge1-network-function-auto-namespace-a* package is not deployed on the edge1 cluster yet:
```
export KUBECONFIG=~/.kube/kind-edge1-config

kubectl get pod -n edge1-network-function-auto-namespace-a
No resources found in network-function-auto-namespace-a namespace.

```

We now propose and approve the deployment package, which merges the package to the *edge1* repository and further triggers
configsync to apply the package to the edge1 cluster.

```
export KUBECONFIG=~/.kube/kind-management-config

porchctl -n porch-demo rpkg propose edge1-48997da49ca0a733b0834c1a27943f1a0e075180
edge1-48997da49ca0a733b0834c1a27943f1a0e075180 proposed

porchctl -n porch-demo rpkg approve edge1-48997da49ca0a733b0834c1a27943f1a0e075180
edge1-48997da49ca0a733b0834c1a27943f1a0e075180 approved

porchctl -n porch-demo rpkg get --name edge1-network-function-auto-namespace-a
NAME                                             PACKAGE                                   WORKSPACENAME   REVISION   LATEST   LIFECYCLE   REPOSITORY
edge1-48997da49ca0a733b0834c1a27943f1a0e075180   edge1-network-function-auto-namespace-a   v1              v1         true     Published   edge1
```

We can now check that the *network-function-auto-namespace-a* package is deployed on the edge1 cluster and that the pod is running
```
export KUBECONFIG=~/.kube/kind-edge1-config

kubectl get pod -n edge1-network-function-auto-namespace-a
No resources found in network-function-auto-namespace-a namespace.

kubectl get pod -n edge1-network-function-auto-namespace-a
NAME                                               READY   STATUS              RESTARTS   AGE
network-function-auto-namespace-85bc658d67-rbzt6   1/1     ContainerCreating   0          3s

kubectl get pod -n edge1-network-function-auto-namespace-a
NAME                                               READY   STATUS    RESTARTS   AGE
network-function-auto-namespace-85bc658d67-rbzt6   1/1     Running   0          10s
```

## Deploying using Package Variant Sets

### Simple PackageVariantSet

The PackageVariant CR is defined as:

```yaml
apiVersion: config.porch.kpt.dev/v1alpha2
kind: PackageVariantSet

metadata:
  name: network-function
  namespace: porch-demo

spec:
  upstream:
    repo: management
    package: network-function
    revision: v1
  targets:
  -  repositories:
      - name: edge1
        packageNames:
        - network-function-b
        - network-function-c
```

In this very simple PackageVariant, the *network-function* package in the *management* repository is cloned into the *edge1*
repository as the *network-function-b* and *network-function-c* package variants.

{{% alert title="Note" color="primary" %}}

This simple package variant does not specify any configuration changes. Normally, as well as cloning and renaming,
configuration changes would be applied on a package variant.

Use `kubectl explain PackageVariantSet` to get help on the structure of the PackageVariantSet CRD.

{{% /alert %}}

Applying the PackageVariantSet creates the new packages as draft packages:

```bash
kubectl apply -f simple-variant.yaml

kubectl get PackageRevisions -n porch-demo | grep -v 'external-blueprints'
NAME                                                           PACKAGE              WORKSPACENAME      REVISION   LATEST   LIFECYCLE   REPOSITORY
edge1-bc8294d121360ad305c9a826a8734adcf5f1b9c0                 network-function-a   v1                 main       false    Published   edge1
edge1-9b4b4d99c43b5c5c8489a47bbce9a61f79904946                 network-function-a   v1                 v1         true     Published   edge1
edge1-a31b56c7db509652f00724dd49746660757cd98a                 network-function-b   packagevariant-1              false    Draft       edge1
edge1-ee14f7ce850ddb0a380cf201d86f48419dc291f4                 network-function-c   packagevariant-1              false    Draft       edge1
management-49580fc22bcf3bf51d334a00b6baa41df597219e            network-function     v1                 main       false    Published   management
management-8b80738a6e0707e3718ae1db3668d0b8ca3f1c82            network-function     v1                 v1         true     Published   management

porchctl -n porch-demo rpkg get --name network-function-b
NAME                                             PACKAGE              WORKSPACENAME      REVISION   LATEST   LIFECYCLE   REPOSITORY
edge1-a31b56c7db509652f00724dd49746660757cd98a   network-function-b   packagevariant-1              false    Draft       edge1

porchctl -n porch-demo rpkg get --name network-function-c
NAME                                             PACKAGE              WORKSPACENAME      REVISION   LATEST   LIFECYCLE   REPOSITORY
edge1-ee14f7ce850ddb0a380cf201d86f48419dc291f4   network-function-c   packagevariant-1              false    Draft       edge1
```

We can see that our two new packages are created as draft packages on the *edge1* repository. We can also examine the
*PacakgeVariant* CRs that have been created:

```bash
kubectl get PackageVariant -n porch-demo
NAMESPACE                      NAME                                                READY   STATUS    RESTARTS        AGE
network-function-a             network-function-9779fc9f5-2tswc                    1/1     Running   0               19h
network-function-b             network-function-9779fc9f5-6zwhh                    1/1     Running   0               76s
network-function-c             network-function-9779fc9f5-h7nsb                    1/1     Running   0               41s
```


It is also interesting to examine the YAML of the *PackageVariant*:

```yaml
kubectl get PackageVariant -n porch-demo -o yaml
apiVersion: v1
items:
- apiVersion: config.porch.kpt.dev/v1alpha1
  kind: PackageVariant
  metadata:
    creationTimestamp: "2024-01-09T15:00:00Z"
    finalizers:
    - config.porch.kpt.dev/packagevariants
    generation: 1
    labels:
      config.porch.kpt.dev/packagevariantset: a923d4fc-a3a7-437c-84d1-52b30dd6cf49
    name: network-function-edge1-network-function-b
    namespace: porch-demo
    ownerReferences:
    - apiVersion: config.porch.kpt.dev/v1alpha2
      controller: true
      kind: PackageVariantSet
      name: network-function
      uid: a923d4fc-a3a7-437c-84d1-52b30dd6cf49
    resourceVersion: "237053"
    uid: 7a81099c-5a0b-49d8-b73c-48e33cd134e5
  spec:
    downstream:
      package: network-function-b
      repo: edge1
    upstream:
      package: network-function
      repo: management
      revision: v1
  status:
    conditions:
    - lastTransitionTime: "2024-01-09T15:00:00Z"
      message: all validation checks passed
      reason: Valid
      status: "False"
      type: Stalled
    - lastTransitionTime: "2024-01-09T15:00:31Z"
      message: successfully ensured downstream package variant
      reason: NoErrors
      status: "True"
      type: Ready
    downstreamTargets:
    - name: edge1-a31b56c7db509652f00724dd49746660757cd98a
- apiVersion: config.porch.kpt.dev/v1alpha1
  kind: PackageVariant
  metadata:
    creationTimestamp: "2024-01-09T15:00:00Z"
    finalizers:
    - config.porch.kpt.dev/packagevariants
    generation: 1
    labels:
      config.porch.kpt.dev/packagevariantset: a923d4fc-a3a7-437c-84d1-52b30dd6cf49
    name: network-function-edge1-network-function-c
    namespace: porch-demo
    ownerReferences:
    - apiVersion: config.porch.kpt.dev/v1alpha2
      controller: true
      kind: PackageVariantSet
      name: network-function
      uid: a923d4fc-a3a7-437c-84d1-52b30dd6cf49
    resourceVersion: "237056"
    uid: da037d0a-9a7a-4e85-842c-1324e9da819a
  spec:
    downstream:
      package: network-function-c
      repo: edge1
    upstream:
      package: network-function
      repo: management
      revision: v1
  status:
    conditions:
    - lastTransitionTime: "2024-01-09T15:00:01Z"
      message: all validation checks passed
      reason: Valid
      status: "False"
      type: Stalled
    - lastTransitionTime: "2024-01-09T15:00:31Z"
      message: successfully ensured downstream package variant
      reason: NoErrors
      status: "True"
      type: Ready
    downstreamTargets:
    - name: edge1-ee14f7ce850ddb0a380cf201d86f48419dc291f4
kind: List
metadata:
  resourceVersion: ""
```

We now want to customize and deploy our two packages. To do this we must pull the packages locally, render the *kpt*
functions, and then push the rendered packages back up to the *edge1* repository.

```bash
porchctl rpkg pull edge1-a31b56c7db509652f00724dd49746660757cd98a tmp_packages_for_deployment/edge1-network-function-b --namespace=porch-demo
kpt fn eval --image=gcr.io/kpt-fn/set-namespace:v0.4.1 tmp_packages_for_deployment/edge1-network-function-b -- namespace=network-function-b
porchctl rpkg push edge1-a31b56c7db509652f00724dd49746660757cd98a tmp_packages_for_deployment/edge1-network-function-b --namespace=porch-demo

porchctl rpkg pull edge1-ee14f7ce850ddb0a380cf201d86f48419dc291f4 tmp_packages_for_deployment/edge1-network-function-c --namespace=porch-demo
kpt fn eval --image=gcr.io/kpt-fn/set-namespace:v0.4.1 tmp_packages_for_deployment/edge1-network-function-c -- namespace=network-function-c
porchctl rpkg push edge1-ee14f7ce850ddb0a380cf201d86f48419dc291f4 tmp_packages_for_deployment/edge1-network-function-c --namespace=porch-demo
```

Check that the namespace has been updated on the two packages in the *edge1* repository using the Gitea web UI.

Now our two packages are ready for deployment:

```bash
porchctl rpkg propose edge1-a31b56c7db509652f00724dd49746660757cd98a --namespace=porch-demo
edge1-a31b56c7db509652f00724dd49746660757cd98a proposed

porchctl rpkg approve edge1-a31b56c7db509652f00724dd49746660757cd98a --namespace=porch-demo
edge1-a31b56c7db509652f00724dd49746660757cd98a approved

porchctl rpkg propose edge1-ee14f7ce850ddb0a380cf201d86f48419dc291f4 --namespace=porch-demo
edge1-ee14f7ce850ddb0a380cf201d86f48419dc291f4 proposed

porchctl rpkg approve edge1-ee14f7ce850ddb0a380cf201d86f48419dc291f4 --namespace=porch-demo
edge1-ee14f7ce850ddb0a380cf201d86f48419dc291f4 approved
```

We can now check that the *network-function-b* and *network-function-c* packages are deployed on the edge1 cluster and
that the pods are running

```bash
export KUBECONFIG=~/.kube/kind-edge1-config

kubectl get pod -A | egrep '(NAMESPACE|network-function)'
NAMESPACE                      NAME                                                READY   STATUS    RESTARTS        AGE
network-function-a             network-function-9779fc9f5-2tswc                    1/1     Running   0               19h
network-function-b             network-function-9779fc9f5-6zwhh                    1/1     Running   0               76s
network-function-c             network-function-9779fc9f5-h7nsb                    1/1     Running   0               41s
```

### Using a PackageVariantSet to automatically set the package name and package namespace

The *PackageVariant* CR defined as:

```yaml
apiVersion: config.porch.kpt.dev/v1alpha2
kind: PackageVariantSet
metadata:
  name: network-function-auto-namespace
  namespace: porch-demo
spec:
  upstream:
    repo: management
    package: network-function-auto-namespace
    revision: v1
  targets:
  - repositories:
    - name: edge1
      packageNames:
      - network-function-auto-namespace-x
      - network-function-auto-namespace-y
    template:
      downstream:
        packageExpr: "target.package + '-cumulonimbus'"
```


In this *PackageVariant*, the *network-function-auto-namespace* package in the *management* repository is cloned into the
*edge1* repository as the *network-function-auto-namespace-x* and *network-function-auto-namespace-y* package variants,
similar to the *PackageVariant* in *simple-variant.yaml*.

An extra template section provided for the repositories in the PackageVariant:

```yaml
template:
  downstream:
    packageExpr: "target.package + '-cumulus'"
```

This template means that each package in the spec.targets.repositories..packageNames list will have the suffix
-cumulus added to its name. This allows us to automatically generate unique package names. Applying the
*PackageVariantSet* also automatically sets a unique namespace for each network function because applying the
*PackageVariantSet* automatically triggers the Kpt pipeline in the *network-function-auto-namespace* *Kpt* package to
generate unique namespaces for each deployed package.

{{% alert title="Note" color="primary" %}}

Many other mutations can be performed using a *PackageVariantSet*. Use `kubectl explain PackageVariantSet` to get help on
the structure of the *PackageVariantSet* CRD to see the various mutations that are possible.

{{% /alert %}}

Applying the *PackageVariantSet* creates the new packages as draft packages:

```bash
kubectl apply -f name-namespace-variant.yaml 
packagevariantset.config.porch.kpt.dev/network-function-auto-namespace created

kunectl get -n porch-demo PackageVariantSet network-function-auto-namespace
NAME                              AGE
network-function-auto-namespace   38s

kubectl get PackageRevisions -n porch-demo | grep auto-namespace
edge1-1f521f05a684adfa8562bf330f7bc72b50e21cc5                 edge1-network-function-auto-namespace-a          v1                 main       false    Published   edge1
edge1-48997da49ca0a733b0834c1a27943f1a0e075180                 edge1-network-function-auto-namespace-a          v1                 v1         true     Published   edge1
edge1-009659a8532552b86263434f68618554e12f4f7c                 network-function-auto-namespace-x-cumulonimbus   packagevariant-1              false    Draft       edge1
edge1-77dbfed49b6cb0723b7c672b224de04c0cead67e                 network-function-auto-namespace-y-cumulonimbus   packagevariant-1              false    Draft       edge1
management-f9a6f2802111b9e81c296422c03aae279725f6df            network-function-auto-namespace                  v1                 main       false    Published   management
management-c97bc433db93f2e8a3d413bed57216c2a72fc7e3            network-function-auto-namespace                  v1                 v1         true     Published   management
```

{{% alert title="Note" color="primary" %}}

The suffix `x-cumulonimbus` and `y-cumulonimbus` has been placed on the package names.

{{% /alert %}}

Examine the *edge1* repository on Gitea and you should see two new draft branches.

- drafts/network-function-auto-namespace-x-cumulonimbus/packagevariant-1
- drafts/network-function-auto-namespace-y-cumulonimbus/packagevariant-1

In these packages, you will see that:

1. The package name has been generated as network-function-auto-namespace-x-cumulonimbus and
   network-function-auto-namespace-y-cumulonimbus in all files in the packages
2. The namespace has been generated as network-function-auto-namespace-x-cumulonimbus and
   network-function-auto-namespace-y-cumulonimbus respectively in the *demployment.yaml* files
3. The PackageVariant has set the data.name field as network-function-auto-namespace-x-cumulonimbus and
   network-function-auto-namespace-y-cumulonimbus respectively in the *pckage-context.yaml* files

This has all been performed automatically; we have not had to perform the
`porchctl rpkg pull/kpt fn render/porchctl rpkg push` combination of commands to make these changes as we had to in the
*simple-variant.yaml* case above.

Now, let us explore the packages further:

```bash
porchctl -n porch-demo rpkg get --name network-function-auto-namespace-x-cumulonimbus
NAME                                             PACKAGE                                          WORKSPACENAME      REVISION   LATEST   LIFECYCLE   REPOSITORY
edge1-009659a8532552b86263434f68618554e12f4f7c   network-function-auto-namespace-x-cumulonimbus   packagevariant-1              false    Draft       edge1

porchctl -n porch-demo rpkg get --name network-function-auto-namespace-y-cumulonimbus
NAME                                             PACKAGE                                          WORKSPACENAME      REVISION   LATEST   LIFECYCLE   REPOSITORY
edge1-77dbfed49b6cb0723b7c672b224de04c0cead67e   network-function-auto-namespace-y-cumulonimbus   packagevariant-1              false    Draft       edge1
```

We can see that our two new packages are created as draft packages on the edge1 repository. We can also examine the
*PacakgeVariant* CRs that have been created:

```bash
kubectl get PackageVariant -n porch-demo
NAME                                                              AGE
network-function-auto-namespace-edge1-network-function-35079f9f   3m41s
network-function-auto-namespace-edge1-network-function-d521d2c0   3m41s
network-function-edge1-network-function-b                         38m
network-function-edge1-network-function-c                         38m
```

It is also interesting to examine the YAML of a *PackageVariant*:

```yaml
kubectl get PackageVariant -n porch-demo network-function-auto-namespace-edge1-network-function-35079f9f -o yaml
apiVersion: config.porch.kpt.dev/v1alpha1
kind: PackageVariant
metadata:
  creationTimestamp: "2024-01-24T15:10:19Z"
  finalizers:
  - config.porch.kpt.dev/packagevariants
  generation: 1
  labels:
    config.porch.kpt.dev/packagevariantset: 71edbdff-21c1-45f4-b9cb-6d2ecfc3da4e
  name: network-function-auto-namespace-edge1-network-function-35079f9f
  namespace: porch-demo
  ownerReferences:
  - apiVersion: config.porch.kpt.dev/v1alpha2
    controller: true
    kind: PackageVariantSet
    name: network-function-auto-namespace
    uid: 71edbdff-21c1-45f4-b9cb-6d2ecfc3da4e
  resourceVersion: "404083"
  uid: 5ae69c2d-6aac-4942-b717-918325650190
spec:
  downstream:
    package: network-function-auto-namespace-x-cumulonimbus
    repo: edge1
  upstream:
    package: network-function-auto-namespace
    repo: management
    revision: v1
status:
  conditions:
  - lastTransitionTime: "2024-01-24T15:10:19Z"
    message: all validation checks passed
    reason: Valid
    status: "False"
    type: Stalled
  - lastTransitionTime: "2024-01-24T15:10:49Z"
    message: successfully ensured downstream package variant
    reason: NoErrors
    status: "True"
    type: Ready
  downstreamTargets:
  - name: edge1-009659a8532552b86263434f68618554e12f4f7c
```
Our two packages are ready for deployment:

```bash
porchctl rpkg propose edge1-009659a8532552b86263434f68618554e12f4f7c --namespace=porch-demo
edge1-009659a8532552b86263434f68618554e12f4f7c proposed

porchctl rpkg approve edge1-009659a8532552b86263434f68618554e12f4f7c --namespace=porch-demo
edge1-009659a8532552b86263434f68618554e12f4f7c approved

porchctl rpkg propose edge1-77dbfed49b6cb0723b7c672b224de04c0cead67e --namespace=porch-demo
edge1-77dbfed49b6cb0723b7c672b224de04c0cead67e proposed

porchctl rpkg approve edge1-77dbfed49b6cb0723b7c672b224de04c0cead67e --namespace=porch-demo
edge1-77dbfed49b6cb0723b7c672b224de04c0cead67e approved
```

We can now check that the packages are deployed on the edge1 cluster and that the pods are running

```bash
export KUBECONFIG=~/.kube/kind-edge1-config

kubectl get pod -A | egrep '(NAMESPACE|network-function)'
NAMESPACE                                 NAME                                                READY   STATUS    RESTARTS       AGE
edge1-network-function-a                  network-function-9779fc9f5-87scj                    1/1     Running   1 (2d1h ago)   4d22h
edge1-network-function-auto-namespace-a   network-function-auto-namespace-85bc658d67-rbzt6    1/1     Running   1 (2d1h ago)   4d22h
network-function-b                        network-function-9779fc9f5-twh2g                    1/1     Running   0              45m
network-function-c                        network-function-9779fc9f5-whhr8                    1/1     Running   0              44m

kubectl get pod -A | egrep '(NAMESPACE|network-function)'
NAMESPACE                                        NAME                                                READY   STATUS              RESTARTS       AGE
edge1-network-function-a                         network-function-9779fc9f5-87scj                    1/1     Running             1 (2d1h ago)   4d22h
edge1-network-function-auto-namespace-a          network-function-auto-namespace-85bc658d67-rbzt6    1/1     Running             1 (2d1h ago)   4d22h
network-function-auto-namespace-x-cumulonimbus   network-function-auto-namespace-85bc658d67-86gml    0/1     ContainerCreating   0              1s
network-function-b                               network-function-9779fc9f5-twh2g                    1/1     Running             0              45m
network-function-c                               network-function-9779fc9f5-whhr8                    1/1     Running             0              44m

kubectl get pod -A | egrep '(NAMESPACE|network-function)'
NAMESPACE                                        NAME                                                READY   STATUS    RESTARTS       AGE
edge1-network-function-a                         network-function-9779fc9f5-87scj                    1/1     Running   1 (2d1h ago)   4d22h
edge1-network-function-auto-namespace-a          network-function-auto-namespace-85bc658d67-rbzt6    1/1     Running   1 (2d1h ago)   4d22h
network-function-auto-namespace-x-cumulonimbus   network-function-auto-namespace-85bc658d67-86gml    1/1     Running   0              10s
network-function-b                               network-function-9779fc9f5-twh2g                    1/1     Running   0              45m
network-function-c                               network-function-9779fc9f5-whhr8                    1/1     Running   0              45m

kubectl get pod -A | egrep '(NAMESPACE|network-function)'
NAMESPACE                                        NAME                                                READY   STATUS    RESTARTS       AGE
edge1-network-function-a                         network-function-9779fc9f5-87scj                    1/1     Running   1 (2d1h ago)   4d22h
edge1-network-function-auto-namespace-a          network-function-auto-namespace-85bc658d67-rbzt6    1/1     Running   1 (2d1h ago)   4d22h
network-function-auto-namespace-x-cumulonimbus   network-function-auto-namespace-85bc658d67-86gml    1/1     Running   0              50s
network-function-b                               network-function-9779fc9f5-twh2g                    1/1     Running   0              46m
network-function-c                               network-function-9779fc9f5-whhr8                    1/1     Running   0              45m

kubectl get pod -A | egrep '(NAMESPACE|network-function)'
NAMESPACE                                        NAME                                                READY   STATUS              RESTARTS       AGE
edge1-network-function-a                         network-function-9779fc9f5-87scj                    1/1     Running             1 (2d1h ago)   4d22h
edge1-network-function-auto-namespace-a          network-function-auto-namespace-85bc658d67-rbzt6    1/1     Running             1 (2d1h ago)   4d22h
network-function-auto-namespace-x-cumulonimbus   network-function-auto-namespace-85bc658d67-86gml    1/1     Running             0              51s
network-function-auto-namespace-y-cumulonimbus   network-function-auto-namespace-85bc658d67-tp5m8    0/1     ContainerCreating   0              1s
network-function-b                               network-function-9779fc9f5-twh2g                    1/1     Running             0              46m
network-function-c                               network-function-9779fc9f5-whhr8                    1/1     Running             0              45m

kubectl get pod -A | egrep '(NAMESPACE|network-function)'
NAMESPACE                                        NAME                                                READY   STATUS    RESTARTS       AGE
edge1-network-function-a                         network-function-9779fc9f5-87scj                    1/1     Running   1 (2d1h ago)   4d22h
edge1-network-function-auto-namespace-a          network-function-auto-namespace-85bc658d67-rbzt6    1/1     Running   1 (2d1h ago)   4d22h
network-function-auto-namespace-x-cumulonimbus   network-function-auto-namespace-85bc658d67-86gml    1/1     Running   0              54s
network-function-auto-namespace-y-cumulonimbus   network-function-auto-namespace-85bc658d67-tp5m8    1/1     Running   0              4s
network-function-b                               network-function-9779fc9f5-twh2g                    1/1     Running   0              46m
network-function-c                               network-function-9779fc9f5-whhr8                    1/1     Running   0              45m
```
