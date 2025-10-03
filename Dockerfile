# Copyright 2025 Stas Levchenko
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#     http://www.apache.org/licenses/LICENSE-2.0

FROM golang:1.25-alpine AS builder

RUN apk add --no-cache git make

WORKDIR /app

ARG TARGETOS=linux
ARG TARGETARCH=amd64

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -o event-exporter ./cmd/main.go

FROM alpine:3.22

RUN apk add --no-cache bash curl ca-certificates busybox-extras

WORKDIR /app
COPY --from=builder /app/event-exporter /app/event-exporter

RUN adduser -D -u 10001 exporter
USER exporter

ENTRYPOINT ["/app/event-exporter"]
