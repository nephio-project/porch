# Network Function

## Description
Network Function Blueprint

## Usage

### Fetch the package
```
kpt pkg get $GIT_HOST/$GIT_USERNAME/$GIT_BLUEPRINTS_REPO[@VERSION] network-function
```
Details: https://kpt.dev/reference/cli/pkg/get/

### View package content
```
kpt pkg tree network-function
```
Details: https://kpt.dev/reference/cli/pkg/tree/

### Apply the package
```
kpt live init network-function
kpt live apply network-function --reconcile-timeout=2m --output=table
```
Details: https://kpt.dev/reference/cli/live/
