# Copyright 2022-2026 The kpt and Nephio Authors
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

ARG ALPINE_VERSION=latest

FROM ghcr.io/kptdev/krm-functions-catalog/apply-setters:v0.2.0 AS apply-setters
FROM ghcr.io/kptdev/krm-functions-catalog/apply-setters:v0.1.1 AS apply-setters-v1
FROM ghcr.io/kptdev/krm-functions-catalog/ensure-name-substring:v0.1.1 AS ensure-name-substring
FROM ghcr.io/kptdev/krm-functions-catalog/search-replace:v0.2.0 AS search-replace
FROM ghcr.io/kptdev/krm-functions-catalog/set-annotations:v0.1.4 AS set-annotations
FROM ghcr.io/kptdev/krm-functions-catalog/set-image:v0.1.2 AS set-image
FROM ghcr.io/kptdev/krm-functions-catalog/set-labels:v0.1.5 AS set-labels
FROM ghcr.io/kptdev/krm-functions-catalog/set-namespace:v0.2.0 AS set-namespace
FROM ghcr.io/kptdev/krm-functions-catalog/set-namespace:v0.3.4 AS set-namespace-v3
FROM ghcr.io/kptdev/krm-functions-catalog/set-project-id:v0.2.0 AS set-project-id
FROM ghcr.io/kptdev/krm-functions-catalog/starlark:v0.3.0 AS starlark
FROM ghcr.io/kptdev/krm-functions-catalog/upsert-resource:v0.2.0 AS upsert-resource

FROM golang:1.25.6-alpine AS builder
WORKDIR /go/src

RUN go install github.com/grpc-ecosystem/grpc-health-probe@v0.4.11
RUN cp $GOPATH/bin/grpc-health-probe /grpc-health-probe

COPY . .
RUN cd func ; go build -v -o /server ./server
COPY func/config.yaml /config.yaml

FROM alpine:${ALPINE_VERSION}
RUN addgroup -g 10001 nonroot && \
    adduser -D -G nonroot -u 10001 nonroot

COPY --from=apply-setters          --chown=nonroot:nonroot /usr/local/bin/function /home/nonroot/functions/apply-setters
COPY --from=apply-setters-v1       --chown=nonroot:nonroot /usr/local/bin/function /home/nonroot/functions/apply-setters-v1
COPY --from=ensure-name-substring  --chown=nonroot:nonroot /usr/local/bin/function /home/nonroot/functions/ensure-name-substring
COPY --from=search-replace         --chown=nonroot:nonroot /usr/local/bin/function /home/nonroot/functions/search-replace
COPY --from=set-annotations        --chown=nonroot:nonroot /usr/local/bin/function /home/nonroot/functions/set-annotations
COPY --from=set-image              --chown=nonroot:nonroot /usr/local/bin/function /home/nonroot/functions/set-image
COPY --from=set-labels             --chown=nonroot:nonroot /usr/local/bin/function /home/nonroot/functions/set-labels
COPY --from=set-namespace          --chown=nonroot:nonroot /usr/local/bin/function /home/nonroot/functions/set-namespace
COPY --from=set-namespace-v3       --chown=nonroot:nonroot /usr/local/bin/function /home/nonroot/functions/set-namespace-v3
COPY --from=set-project-id         --chown=nonroot:nonroot /usr/local/bin/function /home/nonroot/functions/set-project-id
COPY --from=starlark               --chown=nonroot:nonroot /usr/local/bin/star     /home/nonroot/functions/starlark
COPY --from=upsert-resource        --chown=nonroot:nonroot /usr/local/bin/function /home/nonroot/functions/upsert-resource
COPY --from=builder                --chown=nonroot:nonroot /server /grpc-health-probe /config.yaml /home/nonroot/

USER nonroot:nonroot
EXPOSE 9445/tcp
ENTRYPOINT [ "/home/nonroot/server", "--config=/home/nonroot/config.yaml", "--functions=/home/nonroot/functions" ]
