# Network Function

## Description
Network Function with automatic namespace Blueprint

## Usage

### Fetch the package
```
kpt pkg get $GIT_HOST/$GIT_USERNAME/$GIT_BLUEPRINTS_REPO[@VERSION] network-function-auto-namespace
```
Details: https://kpt.dev/reference/cli/pkg/get/

### View package content
```
kpt pkg tree network-function-auto-namespace
```
Details: https://kpt.dev/reference/cli/pkg/tree/

### Apply the package
```
kpt live init network-function-auto-namespace
kpt live apply network-function-auto-namespace --reconcile-timeout=2m --output=table
```
Details: https://kpt.dev/reference/cli/live/
