# =============================================================================
# Multi-stage Dockerfile for devops-backend
# Go 1.23+ | Port 52538 | SQLite Database | OIDC Support
# =============================================================================

# -----------------------------------------------------------------------------
# Stage 1: Builder - Build the Go application
# -----------------------------------------------------------------------------
FROM golang:1.23-alpine AS builder

# Install build dependencies
# - git: Required for go mod download with replace directives
# - gcc, musl-dev: Required for CGO (SQLite driver modernc.org/sqlite)
RUN apk add --no-cache \
    git \
    gcc \
    musl-dev \
    ca-certificates

# Set working directory
WORKDIR /build

# Copy go.mod and go.sum first for better layer caching
COPY go.mod go.sum ./

# Download dependencies
# The replace directive in go.mod will automatically use the forked module
# from github.com/Vickko/eino-devops-custom
RUN go mod download && go mod verify

# Copy source code
COPY . .

# Build the application
# CGO_ENABLED=1: Required for modernc.org/sqlite (pure Go SQLite driver with CGO)
# -ldflags: Strip debug information and reduce binary size
# -trimpath: Remove file system paths from binary for reproducibility
RUN CGO_ENABLED=1 GOOS=linux go build \
    -ldflags="-s -w" \
    -trimpath \
    -o /build/server \
    ./cmd/server

# Verify the binary was created successfully
RUN ls -lh /build/server

# -----------------------------------------------------------------------------
# Stage 2: Runtime - Minimal production image
# -----------------------------------------------------------------------------
FROM alpine:3.21

# Install runtime dependencies
# - ca-certificates: For HTTPS connections to AI providers and OIDC
# - tzdata: For proper timezone handling
# - sqlite-libs: Runtime libraries for SQLite
RUN apk add --no-cache \
    ca-certificates \
    tzdata \
    sqlite-libs

# Create non-root user for security
RUN addgroup -g 1000 appuser && \
    adduser -D -u 1000 -G appuser appuser

# Set working directory
WORKDIR /app

# Create necessary directories with proper permissions
RUN mkdir -p /app/data /app/configs /app/logs && \
    chown -R appuser:appuser /app

# Copy binary from builder stage
COPY --from=builder --chown=appuser:appuser /build/server /app/server

# Copy default configuration (can be overridden via volume mount)
COPY --chown=appuser:appuser configs/config.yaml /app/configs/config.yaml

# Switch to non-root user
USER appuser

# Expose application port
EXPOSE 52538

# Health check endpoint (commented out - requires /health endpoint implementation)
# Uncomment if you add a /health endpoint to your API
# HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
#     CMD wget --no-verbose --tries=1 --spider http://localhost:52538/health || exit 1

# Environment variables (can be overridden at runtime)
ENV CONFIG_PATH=/app/configs/config.yaml \
    DB_PATH=/app/data/sessions.db \
    PORT=52538 \
    LOG_LEVEL=info

# Set the entrypoint
ENTRYPOINT ["/app/server"]

# Default command (can override config path)
CMD ["-conf", "/app/configs/config.yaml"]

# -----------------------------------------------------------------------------
# Metadata
# -----------------------------------------------------------------------------
LABEL maintainer="devops-backend team" \
      version="1.0.0" \
      description="DevOps Backend - AI-powered chat service with multi-provider support" \
      org.opencontainers.image.source="https://github.com/Vickko/eino-devops-custom"
