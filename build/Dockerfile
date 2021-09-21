# Builder
# golang:1.17.0-alpine3.14
FROM golang@sha256:c03a922dfb0d6d3a94d46cc5be69002011763595ac2786068fde63ee174d797b as builder

COPY . ./app

WORKDIR ./app

RUN export GO111MODULE=on

RUN go get -v ./...

RUN apk add --no-cache gcc musl-dev libc6-compat

RUN go build -ldflags "-linkmode external -extldflags -static" -o /namespace-node-affinity cmd/nsnodeaffinity/main.go

# Webhook
FROM alpine:latest

EXPOSE 8443

COPY --from=builder /namespace-node-affinity /namespace-node-affinity

ENTRYPOINT ["/namespace-node-affinity"]
