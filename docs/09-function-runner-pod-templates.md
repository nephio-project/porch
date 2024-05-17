# Function runner pod templating

## Why 

`porch-fn-runner` implements a simplistic function-as-a-service for executing kpt functions, running the needed kpt functions wrapped in a grpc server. The function is starting up a number of function evaluator pods for each of the kpt functions. As with any operator that manages pods, it's good to provide some templating and parametrization capabilities of the pods that will be managed by the function runner.

## Contract for writing pod templates

The following contract needs to be fulfilled by any function evaluator pod template:

1. There is a container named "function".
2. The entrypoint of the "function" container will start the wrapper grpc server.
3. The image of the "function" container can be set to the kpt function's image without impacting starting the entrypoint.
4. The arguments of the "function" container can be appended with the entries from the Dockerfile  ENTRYPOINT of the kpt function image.

## Enabling pod templating on function runner

A Configmap with the pod template should be created in the namespace where the porch-fn-runner pod is running.
The configmap's name should be included as `--function-pod-template` in the command line arguments in the pod spec of the function runner.

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


## Example pod template

The below pod template Configmpa matches the default behavior:

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
        image: gcr.io/example-google-project-id/porch-wrapper-server:latest
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
          - /wrapper-server-tools/wrapper-server --port 9446 -- 
        volumeMounts:
          - name: wrapper-server-tools
            mountPath: /wrapper-server-tools
    volumes:
      - name: wrapper-server-tools
        emptyDir: {}
```