# Copyright 2022 The kpt and Nephio Authors
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

FROM gcr.io/kpt-fn/apply-setters:v0.2.0 AS apply-setters
FROM gcr.io/kpt-fn/apply-setters:v0.1.1 AS apply-setters-v1
FROM gcr.io/kpt-fn/ensure-name-substring:v0.1.1 AS ensure-name-substring
FROM gcr.io/kpt-fn/search-replace:v0.2.0 AS search-replace
FROM gcr.io/kpt-fn/set-annotations:v0.1.4 AS set-annotations
FROM gcr.io/kpt-fn/set-image:v0.1.0 AS set-image
FROM gcr.io/kpt-fn/set-labels:v0.1.5 AS set-labels
FROM gcr.io/kpt-fn/set-namespace:v0.2.0 AS set-namespace
FROM gcr.io/kpt-fn/set-namespace:v0.3.4 AS set-namespace-v3
FROM gcr.io/kpt-fn/set-project-id:v0.2.0 AS set-project-id
FROM gcr.io/kpt-fn/starlark:v0.3.0 AS starlark
FROM gcr.io/kpt-fn/upsert-resource:v0.2.0 AS upsert-resource

FROM golang:1.22.1-alpine3.18 AS builder
WORKDIR /go/src

RUN go install github.com/grpc-ecosystem/grpc-health-probe@v0.4.11
RUN cp $GOPATH/bin/grpc-health-probe /grpc-health-probe

COPY . .
RUN cd func ; go build -v -o /server ./server
COPY func/config.yaml /config.yaml

FROM alpine:3.18

COPY --from=apply-setters          /usr/local/bin/function /functions/apply-setters
COPY --from=apply-setters-v1       /usr/local/bin/function /functions/apply-setters-v1
COPY --from=ensure-name-substring  /usr/local/bin/function /functions/ensure-name-substring
COPY --from=search-replace         /usr/local/bin/function /functions/search-replace
COPY --from=set-annotations        /usr/local/bin/function /functions/set-annotations
COPY --from=set-image              /usr/local/bin/function /functions/set-image
COPY --from=set-labels             /usr/local/bin/function /functions/set-labels
COPY --from=set-namespace          /usr/local/bin/function /functions/set-namespace
COPY --from=set-namespace-v3       /usr/local/bin/function /functions/set-namespace-v3
COPY --from=set-project-id         /usr/local/bin/function /functions/set-project-id
COPY --from=starlark               /usr/local/bin/star     /functions/starlark
COPY --from=upsert-resource        /usr/local/bin/function /functions/upsert-resource
COPY --from=builder                /server /grpc-health-probe /config.yaml /

EXPOSE 9445/tcp
ENTRYPOINT [ "/server", "--config=/config.yaml", "--functions=/functions" ]
