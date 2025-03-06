# Copyright 2022-2025 The kpt and Nephio Authors
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

FROM golang:1.23.5-bookworm AS builder

WORKDIR /go/src

# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
COPY go.mod go.sum ./

RUN echo "Downloading porch modules ..." \
 && go mod download

ENV CGO_ENABLED=0
# Prebuild some library dependencies to warm the cache
RUN go build -v \
  google.golang.org/grpc \
  k8s.io/apiserver/pkg/server \
  k8s.io/component-base/cli \
  k8s.io/klog/v2 \
  github.com/google/go-containerregistry/pkg/gcrane \
  k8s.io/client-go/kubernetes/scheme \
  github.com/go-git/go-git/v5 \
  sigs.k8s.io/kustomize/kyaml/...

COPY ./api ./api
COPY ./cmd ./cmd
COPY ./pkg ./pkg
COPY ./internal ./internal
COPY ./controllers ./controllers
COPY ./func ./func
COPY ./third_party ./third_party

WORKDIR /go/src/controllers/
RUN CGO_ENABLED=0 go build -o /porch-controllers -v .

FROM gcr.io/distroless/static
WORKDIR /data
COPY --from=builder /porch-controllers /porch-controllers

ENTRYPOINT ["/porch-controllers"]
