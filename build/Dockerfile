# Builder
# golang:1.18.0-alpine3.15
FROM golang@sha256:e7d1b2dff120cb5037a84e633968cf133b9af7c5f18d10cb150c4f9f7e15849b as builder

COPY . ./app

WORKDIR ./app

RUN go mod download

RUN export GO111MODULE=on

RUN apk add --no-cache gcc musl-dev libc6-compat

RUN go build -ldflags "-linkmode external -extldflags -static" -o /namespace-node-affinity cmd/nsnodeaffinity/main.go

# Webhook
FROM scratch

EXPOSE 8443

COPY --from=builder /namespace-node-affinity /namespace-node-affinity

ENTRYPOINT ["/namespace-node-affinity"]
