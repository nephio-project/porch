# Local forks of 3rd party dependencies

## kptdev

This directory contains a local fork of 3rd party go libraries that porchhas temporarily cloned.

The local copy of "kptdev" will be removed once the local changes in the kptdev package here have been merged into the [kpt project](https://github.com/kptdev)

## Kubernetes API server

The local copy of the Kubernetes API Server is a clone of the kubernetes API server version 0.30.3. The changes are:

- An increase of the API server hard coded timeout from 34 seconds to 290 seconds
```
diff -w /Users/liam/git/github/kubernetes/apiserver/pkg/endpoints/handlers/rest.go apiserver-0.34.1/pkg/endpoints/handlers/rest.go

54c54
< 	requestTimeoutUpperBound = 34 * time.Second
---
> 	requestTimeoutUpperBound = 290 * time.Second
```

- Removal of the "OWNERS" files