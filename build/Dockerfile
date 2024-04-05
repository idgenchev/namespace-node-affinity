# Builder
FROM golang:1.21.9-alpine3.19 as builder

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
