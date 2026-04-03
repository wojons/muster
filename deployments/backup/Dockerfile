FROM alpine:3.19

LABEL maintainer="muster@anomalyco.io"
LABEL description="Muster Backup Container"

# Install required packages
RUN apk add --no-cache \
    bash \
    sqlite \
    openssl \
    curl \
    jq \
    aws-cli \
    coreutils \
    ca-certificates

# Create user
RUN addgroup -g 1000 muster \
    && adduser -D -u 1000 -G muster muster

# Create directories
RUN mkdir -p /backups /data /usr/local/bin /var/lib/muster/metrics \
    && chown -R muster:muster /backups /data /var/lib/muster

# Copy scripts
COPY backup-database.sh /usr/local/bin/
COPY restore-database.sh /usr/local/bin/
COPY verify-backup.sh /usr/local/bin/
COPY backup-monitor.sh /usr/local/bin/

# Set permissions
RUN chmod +x /usr/local/bin/*.sh

# Switch to non-root user
USER muster

# Set environment defaults
ENV BACKUP_DIR=/backups
ENV DB_PATH=/data/muster.db
ENV RETENTION_DAYS=30
ENV ENABLE_COMPRESSION=true
ENV ENABLE_ENCRYPTION=false
ENV NOTIFY_ON_SUCCESS=false
ENV NOTIFY_ON_FAILURE=true

# Health check
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD test -d /backups || exit 1

# Default command
CMD ["/usr/local/bin/backup-database.sh"]
