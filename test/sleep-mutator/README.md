# sleep

Note: Please ensure you follow the [kpt doc style guide].

## Overview

<!--mdtogo:Short-->

Simulates a sleep delay based on the value provided in a ConfigMap field `sleepSeconds`.

<!--mdtogo-->

This function introduces a deliberate delay in a kpt pipeline for debugging or simulation purposes.

It is helpful for use cases such as testing the timing and behavior of orchestrated pipelines where simulating latency is useful. For instance, it can help in evaluating the responsiveness and concurrency of pipeline steps.

[//]: <> (Note: The content between `<!--mdtogo:Short-->` and the following
`<!--mdtogo-->` will be used as the short description for the command.)

<!--mdtogo:Long-->

## Usage

The `sleep` function is a utility that pauses execution for a user-defined number of seconds.

This can be used in both **imperative** (`kpt fn run`) and **declarative** (`functionConfig`) modes.

### FunctionConfig

The function expects a `ConfigMap` with a `data.sleepSeconds` field to define how long to sleep.

- **sleepSeconds**
    - *Type:* string
    - *Example:* `"5"`
    - *Optional:* Yes
    - *Default:* `"10"`
    - *Description:* Number of seconds the function will pause execution.

Example FunctionConfig:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: sleep-config
data:
  sleepSeconds: "5"
```

[//]: <> (Note: The content between `<!--mdtogo:Long-->` and the following
`<!--mdtogo-->` will be used as the long description for the command.)

<!--mdtogo-->

## Examples

<!--mdtogo:Examples-->

### Run imperatively

```sh
kpt fn run . --image gcr.io/kpt-fn/sleep --fn-config path/to/config.yaml
```

### Example Output

The function will log output like:

```
Sleeping for 5 seconds...
Sleep completed.
```

[//]: <> (Note: The content between `<!--mdtogo:Examples-->` and the following
`<!--mdtogo-->` will be used as the examples for the command.)

<!--mdtogo-->

[kpt doc style guide]: https://github.com/kptdev/kpt/blob/main/docs/style-guides/docs.md
