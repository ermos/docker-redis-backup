# Build stage
FROM golang:1.23-alpine AS builder

WORKDIR /app

# Install git and ca-certificates (needed for go mod download and HTTPS)
RUN apk add --no-cache git ca-certificates tzdata

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /redis-backup .

# Final stage
FROM alpine:3.20

# Install ca-certificates for HTTPS and tzdata for timezone support
RUN apk add --no-cache ca-certificates tzdata

# Create necessary directories
RUN mkdir -p /backups /data

# Copy binary from builder
COPY --from=builder /redis-backup /usr/local/bin/redis-backup

# Default environment variables (non-sensitive only)
ENV REDIS_HOST=localhost \
    REDIS_PORT=6379 \
    REDIS_DB=0 \
    BACKUP_CRON="0 0 * * *" \
    BACKUP_ON_START=false \
    STORAGE_TYPE=local \
    LOCAL_BACKUP_PATH=/backups \
    REDIS_DATA_PATH=/data \
    S3_REGION=us-east-1 \
    S3_PATH_STYLE=false \
    S3_BACKUP_PREFIX=redis-backups \
    RETENTION_COUNT=0

# Volume for local backups
VOLUME ["/backups", "/data"]

ENTRYPOINT ["/usr/local/bin/redis-backup"]
