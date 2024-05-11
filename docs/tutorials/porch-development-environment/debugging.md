# Prerequisites
1. [kind](https://kind.sigs.k8s.io/)
2. [docker](https://www.docker.com/)
3. [go](https://go.dev/)
4. [ctlptl](https://github.com/tilt-dev/ctlptl)
5. [tilt](https://github.com/tilt-dev/tilt)
6. [VS Code](https://code.visualstudio.com/)


# Introduction

This guide provides a description on how to run debuggable versions of the porch pods.

This guide does not provide details on how to set up a git distribution, or how to configure porch to connect to it.

## Components

1. **Kind** is used as an ephemeral cluster distribution. Note that in the below instructions the `kind` cli is not directly used for creating a cluster, but it is a prerequisite of `ctlptl`.
2. **Docker** is used to build the normal and the debug-enabled images and used for hosting the kind cluster and it's associated registry. Other container builders/runtimes might work, but weren't tested.
3. **go** is needed for linting as a dependency of VS Code. The host's go version is not used during actual image builds.
4. **ctlptl** is used for setting up a local container registry and a kind cluster.
5. **tilt** is used for deploying k8s manifests, triggering docker image builds and replacement of images in already pushed manifests.
6. **VS Code** is used for it's remote debugger feature to allow stepping through the code.

# Instructions

## Deploying a cluster
Execute the following command to get a kind cluster created with a pre-configured container registry. This will result in 2 containers running, one will be the kind cluster, the other will be the container registry, which will be available on a random port on the host.

```
ctlptl create cluster kind --name kind-porch --registry ctlptl-registry
```

## Creating a tilt-config file

In the root directory of the porch project, `tilt_config.json` file should be created, that will control settings related to tilt.
```json
{
    "porch-server-debug": false,
    "porch-controllers-debug": false,
    "porch-function-runner-debug": false
}
```
Currently it has 3 options in it, each of them controlling whether the debug or the non-debug container image should be deployed for the given component.

## Starting tilt

After the tilt-config file is created, tilt should be started with the `tilt up` command. 
This will deploy the porch components into the kind cluster. It will also generate a web UI that's available on a port of the host machine.

There are 2 views in the web UI, table, and details. It's better to look at details view, because the docker build and the microservice logs are visible there. 

Through this web UI, rebuilds and redeployments of each of the porch components can be triggered by clicking the reload button beside the name of the respective component.
Note that the current build times of the non-debug containers are quite bad, as there is no proper incremental builds.

## Debugging one of the services

As a first step, in the `tilt_config.json` file, the correspondig entry should be set to true.
Then on the web UI, the reload button should be pressed for the respective service.
When the image with the debug enabled is started up, the container will log something like the following:
`info layer=debugger launching process with args:`. This means the debugger wrapper is up, and waiting for a connection. 
At this point, open VS code, and select the "Remote Porch Server", "Remote Porch Controllers" or "Remote Porch Function Runner", depending on the component.
This will attach the VS code instance to the container, and the container process will start up. It's possible to debug startup issues this way, as the debugger server won't start the porch processes until there is a debugger client (VS Code) attached to it.

### What's happening underneath
For each of the porch components there is a `Dockerfile.debug`, which has 2 significant differences from the original process.
The application code is compiled with flags so that all inlining of code and other optimalizations done by the compiler are disabled, so the debugger can track source code lines correctly.
A wrapper application is replacing the entrypoint of the container, called [delve](https://github.com/go-delve/delve). This will expose a rpc API called [dap](https://microsoft.github.io/debug-adapter-protocol/overview), which can be used by many IDEs for remote debugging. The ports `:4000`, `:4001`, `:4002` are hardcoded into the respective images. Delve will wait until there is a debugger attached before kicking off the porch application process.
There is a configuration in `Tiltfile`, which whenever one of the containers are deployed in debug mode, creates a `kubectl port-forward` for the respective port to `localhost`. This means that VS Code can now attach it's remote debugger to the `localhost:400x` port, and start monitoring the running application in k8s.