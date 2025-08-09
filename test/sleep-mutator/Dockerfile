ARG DOCKERHUB_MIRROR=""
FROM ${DOCKERHUB_MIRROR}golang:1.23 AS build
ENV CGO_ENABLED=0
WORKDIR /app

COPY sleep-mutator/go.mod sleep-mutator/go.sum ./

RUN go mod download -x

COPY sleep-mutator/ .
RUN go build -o /usr/local/bin/sleep .

#############################################

FROM ${DOCKERHUB_MIRROR}alpine:3.21
COPY --from=build /usr/local/bin/sleep /usr/local/bin/function
ENTRYPOINT ["sleep"]
