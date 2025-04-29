# Iterative Performance Test

## Setup

Run `prometheus_in_docker.sh create`, this will create a docker network so that the container can reach the metrics server,
then starts a container of Prometheus reachable at `localhost:9090`.

## Running

You can see all arguments with their default values by running `go test -v ./... -help=true` in this directory.

The `PERF` environment variable must be set to `1` in order for the test to start.

### Examples

`PERF=1 go test -v ./... -repos 1 -iterations 10,20,30` -
run 3 iterations, creating 10, then 20, then 30 control PackageRevisions in 1 Repository

`PERF=1 go test -v ./... -repos 3 -iterations 10,20,10,30` -
run 4 iterations, creating 30 (10x3), then 60 (20x3), then 30 (10x3), then 90 (30*3) PackageRevisions across 3 Repositories

## Teardown

Run `prometheus_in_docker.sh clean`, this will delete the container and the network.

## Example queries for Prometheus

### propose (k8s client update) latency averaged over 1s + actual PR count

```
rate(porch_operation_duration_ms_sum{operation="propose"}[1s])/rate(porch_operation_duration_ms_count{operation="propose"}[1s])
or
porch_package_revisions_count
```

### all k8s client operation latency averaged over 1s for control packages + actual PR count

```
rate(porch_operation_duration_ms_sum{name=~"iterative-.*"}[1s])/rate(porch_operation_duration_ms_count{name=~"iterative-.*"}[1s])
or
porch_package_revisions_count
```
