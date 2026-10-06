# ---------- Builder ----------
FROM golang:1.23-alpine3.20 AS builder

WORKDIR /src

# Upgrade all packages to latest patched versions
RUN apk update && \
    apk upgrade --no-cache && \
    apk add --no-cache git ca-certificates tzdata

COPY go.mod go.sum* ./
ENV GOTOOLCHAIN=local
RUN go mod download && go mod verify

COPY . .

ENV CGO_ENABLED=0 GOOS=linux GOARCH=amd64

RUN go build -trimpath -ldflags="-s -w" -o /out/backend   ./cmd/backend && \
    go build -trimpath -ldflags="-s -w" -o /out/publisher ./cmd/publisher && \
    go build -trimpath -ldflags="-s -w" -o /out/worker    ./cmd/worker

# ---------- Base runtime ----------
FROM alpine:3.20 AS base

RUN apk update && \
    apk upgrade --no-cache && \
    apk add --no-cache ca-certificates tzdata && \
    addgroup -g 1000 -S app && \
    adduser -u 1000 -S app -G app

WORKDIR /app

COPY --from=builder /out/backend   /app/backend
COPY --from=builder /out/publisher /app/publisher
COPY --from=builder /out/worker    /app/worker
COPY configs/ /app/configs/

USER app:app

# ---------- Backend ----------
FROM base AS backend
EXPOSE 8080
ENTRYPOINT ["/app/backend"]

# ---------- Worker ----------
FROM base AS worker
ENTRYPOINT ["/app/worker"]

# ---------- Publisher ----------
FROM base AS publisher
ENTRYPOINT ["/app/publisher"]