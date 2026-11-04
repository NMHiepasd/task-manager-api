# ==========================================
# Stage 1: Build Application Binary
# ==========================================
FROM golang:alpine AS builder

WORKDIR /build

# Build arguments
ARG VERSION=1.0.0

# Install build dependencies
RUN apk add --no-cache git

# Cache Go module dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Compile optimized static binary with injected version
RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -ldflags="-s -w -X main.version=${VERSION}" \
    -o /build/bin/server ./cmd/api

# ==========================================
# Stage 2: Minimal Production Runtime
# ==========================================
FROM alpine:3.21

WORKDIR /app

# Install security certificates and timezone data
RUN apk --no-cache add ca-certificates tzdata

# Create secure non-root user and group
RUN addgroup -S appgroup && adduser -S appuser -G appgroup

# Copy compiled binary from builder
COPY --from=builder --chown=appuser:appgroup /build/bin/server /app/server
COPY --from=builder --chown=appuser:appgroup /build/.env.example /app/.env.example

# Run as non-root user
USER appuser:appgroup

# Expose HTTP port
EXPOSE 8080

# Container liveness health check
HEALTHCHECK --interval=10s --timeout=3s --start-period=5s --retries=3 \
    CMD wget -qO- http://localhost:8080/healthz || exit 1

ENTRYPOINT ["/app/server"]
