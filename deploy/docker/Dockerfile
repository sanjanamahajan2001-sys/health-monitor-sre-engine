# Multi-stage build for production
FROM golang:1.21-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git ca-certificates

# Set working directory
WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags='-w -s -extldflags "-static"' \
    -a -installsuffix cgo \
    -o health-monitor \
    ./cmd/health-monitor

# Production stage
FROM alpine:3.18

# Install runtime dependencies
RUN apk add --no-cache \
    ca-certificates \
    wget \
    xclip \
    && rm -rf /var/cache/apk/*

# Create non-root user
RUN addgroup -g 1000 health-monitor && \
    adduser -D -s /bin/sh -u 1000 -G health-monitor health-monitor

# Set working directory
WORKDIR /opt/health-monitor

# Copy binary from builder
COPY --from=builder /app/health-monitor .

# Create directories with proper permissions
RUN mkdir -p /var/lib/health-monitor /var/log/health-monitor /etc/health-monitor && \
    chown -R health-monitor:health-monitor /var/lib/health-monitor /var/log/health-monitor /opt/health-monitor && \
    chmod 755 /var/lib/health-monitor /var/log/health-monitor /opt/health-monitor

# Switch to non-root user
USER health-monitor

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=10s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/healthz || exit 1

# Expose ports
EXPOSE 8080 9090

# Set entrypoint
ENTRYPOINT ["./health-monitor", "--foreground"]

# Default command
CMD ["--log-level", "info", "--log-format", "json"]
