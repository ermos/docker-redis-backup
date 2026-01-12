# Docker Redis Backup

A Go application running in Docker that creates Redis backups on a configurable schedule. Supports local storage, S3-compatible storage providers, and Google Cloud Storage with native API.

## Features

- Scheduled backups using cron expressions (robfig/cron)
- Triggers Redis BGSAVE for consistent snapshots
- Local filesystem storage
- S3-compatible storage (AWS, MinIO, DigitalOcean, Cloudflare R2)
- Google Cloud Storage with Service Account (native API)
- Configurable backup retention
- Optional backup on startup
- Environment variable configuration
- Lightweight Alpine-based Docker image

## Quick Start

### Local Storage

```bash
docker-compose up -d redis redis-backup-local
```

### S3/MinIO Storage

```bash
docker-compose --profile s3 up -d
```

## Configuration

All configuration is done via environment variables:

### Redis Configuration

| Variable | Description | Default |
|----------|-------------|---------|
| `REDIS_HOST` | Redis server hostname | `localhost` |
| `REDIS_PORT` | Redis server port | `6379` |
| `REDIS_PASSWORD` | Redis password | (empty) |
| `REDIS_DB` | Redis database number | `0` |
| `REDIS_DATA_PATH` | Path to Redis data directory (where dump.rdb is located) | `/data` |

### Backup Configuration

| Variable | Description | Default |
|----------|-------------|---------|
| `BACKUP_CRON` | Cron expression for backup schedule | **Required** |
| `BACKUP_ON_START` | Run backup when service starts | `false` |
| `RETENTION_COUNT` | Number of backups to keep (0 = unlimited) | `0` |

### Storage Configuration

| Variable | Description | Default |
|----------|-------------|---------|
| `STORAGE_TYPE` | Storage type: `local`, `s3`, or `gcp` | `local` |
| `LOCAL_BACKUP_PATH` | Path for local backups | `/backups` |

### S3 Configuration

| Variable | Description | Default |
|----------|-------------|---------|
| `S3_ENDPOINT` | S3 endpoint URL (leave empty for AWS) | (empty) |
| `S3_REGION` | S3 region | `us-east-1` |
| `S3_BUCKET` | S3 bucket name | **Required for S3** |
| `S3_ACCESS_KEY` | S3 access key | (empty) |
| `S3_SECRET_KEY` | S3 secret key | (empty) |
| `S3_PATH_STYLE` | Use path-style URLs (required for MinIO) | `false` |
| `S3_BACKUP_PREFIX` | Prefix/folder in bucket | (empty) |

### GCP Cloud Storage Configuration

| Variable | Description | Default |
|----------|-------------|---------|
| `GCS_BUCKET` | GCS bucket URI (format: `gs://bucket-name/prefix`) | **Required for GCP** |
| `GCP_CREDENTIALS_FILE` | Path to service account JSON file | (empty) |

## Cron Expression Examples

| Expression | Description |
|------------|-------------|
| `0 0 * * *` | Every day at midnight |
| `0 */6 * * *` | Every 6 hours |
| `0 0 * * 0` | Every Sunday at midnight |
| `*/30 * * * *` | Every 30 minutes |
| `0 2 * * *` | Every day at 2 AM |

## Provider Examples

### AWS S3

```yaml
environment:
  - STORAGE_TYPE=s3
  - S3_REGION=us-east-1
  - S3_BUCKET=my-redis-backups
  - S3_ACCESS_KEY=AKIAIOSFODNN7EXAMPLE
  - S3_SECRET_KEY=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY
```

### MinIO

```yaml
environment:
  - STORAGE_TYPE=s3
  - S3_ENDPOINT=http://minio:9000
  - S3_BUCKET=redis-backups
  - S3_ACCESS_KEY=minioadmin
  - S3_SECRET_KEY=minioadmin
  - S3_PATH_STYLE=true
```

### Google Cloud Storage (Service Account - Recommended)

```yaml
environment:
  - STORAGE_TYPE=gcp
  - GCS_BUCKET=gs://my-bucket/redis/backups
  - GCP_CREDENTIALS_FILE=/credentials/service-account.json
volumes:
  - ./credentials:/credentials:ro
```

### Google Cloud Storage (HMAC Keys)

```yaml
environment:
  - STORAGE_TYPE=s3
  - S3_ENDPOINT=https://storage.googleapis.com
  - S3_REGION=auto
  - S3_BUCKET=my-gcs-bucket
  - S3_ACCESS_KEY=GOOGEXAMPLE
  - S3_SECRET_KEY=your-hmac-secret
```

### DigitalOcean Spaces

```yaml
environment:
  - STORAGE_TYPE=s3
  - S3_ENDPOINT=https://nyc3.digitaloceanspaces.com
  - S3_REGION=nyc3
  - S3_BUCKET=my-space
  - S3_ACCESS_KEY=your-access-key
  - S3_SECRET_KEY=your-secret-key
```

### Cloudflare R2

```yaml
environment:
  - STORAGE_TYPE=s3
  - S3_ENDPOINT=https://account-id.r2.cloudflarestorage.com
  - S3_REGION=auto
  - S3_BUCKET=my-bucket
  - S3_ACCESS_KEY=your-access-key
  - S3_SECRET_KEY=your-secret-key
  - S3_PATH_STYLE=true
```

## Docker Compose Example

```yaml
services:
  redis:
    image: redis:7-alpine
    volumes:
      - redis-data:/data

  redis-backup:
    image: ghcr.io/ermos/docker-redis-backup:latest
    environment:
      - REDIS_HOST=redis
      - BACKUP_CRON=0 0 * * *
      - STORAGE_TYPE=local
      - RETENTION_COUNT=7
    volumes:
      - redis-data:/data:ro
      - ./backups:/backups

volumes:
  redis-data:
```

## Building

```bash
# Build Docker image
docker build -t redis-backup .

# Build locally
go build -o redis-backup .
```

## How It Works

1. The service connects to Redis and starts a cron scheduler
2. At scheduled times (or on startup if configured):
   - Triggers Redis `BGSAVE` command
   - Waits for the background save to complete
   - Copies the `dump.rdb` file to the configured storage
   - Applies retention policy (deletes old backups if configured)

## License

MIT
