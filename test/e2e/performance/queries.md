# Example queries

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
