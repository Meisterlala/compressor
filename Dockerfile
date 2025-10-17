# Build stage
FROM golang:1.21-alpine AS builder

WORKDIR /app

# Copy go mod
COPY go.mod go.sum ./
RUN go mod download

# Copy source
COPY . .

# Build
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o compressor ./cmd/compressor

# Runtime stage with CUDA support
FROM nvidia/cuda:11.8-runtime-ubuntu20.04

# Install ffmpeg and ca-certificates
RUN apt-get update && apt-get install -y ffmpeg ca-certificates && rm -rf /var/lib/apt/lists/*

# Create app user
RUN groupadd -r appgroup && useradd -r -g appgroup appuser

# Copy binary
COPY --from=builder /app/compressor /usr/local/bin/compressor

# Create directories
RUN mkdir -p /input /output && chown -R appuser:appgroup /input /output

USER appuser

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
  CMD curl -f http://localhost:8080/status || exit 1

# Default command
CMD ["/usr/local/bin/compressor"]