# Docker Compose Specification

**Version:** 1.0.0  
**Last Updated:** 2026-02-28

---

## Development Environment

```yaml
# docker-compose.dev.yaml
version: '3.8'

services:
  muster:
    build:
      context: .
      dockerfile: Dockerfile.dev
    container_name: muster-dev
    volumes:
      - .:/app
      - go-cache:/go/pkg/mod
      - go-build:/root/.cache/go-build
    working_dir: /app
    command: ["air", "-c", ".air.toml"]
    environment:
      - ENV=development
      - LOG_LEVEL=debug
    ports:
      - "8080:8080"
    networks:
      - muster-network

  # Hot reload for Go
  air:
    image: cosmtrek/air:latest
    volumes:
      - .:/app
    working_dir: /app

volumes:
  go-cache:
  go-build:

networks:
  muster-network:
    driver: bridge
```

---

## PostgreSQL Service

```yaml
  postgres:
    image: postgres:16-alpine
    container_name: muster-postgres
    environment:
      POSTGRES_USER: muster
      POSTGRES_PASSWORD: muster_dev
      POSTGRES_DB: muster_test
    volumes:
      - postgres-data:/var/lib/postgresql/data
      - ./scripts/init-db.sql:/docker-entrypoint-initdb.d/init.sql
    ports:
      - "5432:5432"
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U muster"]
      interval: 5s
      timeout: 5s
      retries: 5
```

---

## MySQL Service

```yaml
  mysql:
    image: mysql:8.0
    container_name: muster-mysql
    environment:
      MYSQL_ROOT_PASSWORD: root_dev
      MYSQL_DATABASE: muster_test
      MYSQL_USER: muster
      MYSQL_PASSWORD: muster_dev
    volumes:
      - mysql-data:/var/lib/mysql
      - ./scripts/init-mysql.sql:/docker-entrypoint-initdb.d/init.sql
    ports:
      - "3306:3306"
    command: --default-authentication-plugin=mysql_native_password
    healthcheck:
      test: ["CMD", "mysqladmin", "ping", "-h", "localhost"]
      interval: 5s
      timeout: 5s
      retries: 5
```

---

## Volume Mounts

```yaml
volumes:
  # Source code (read-write for dev)
  - type: bind
    source: .
    target: /app
    consistency: delegated

  # Go module cache (performance)
  go-mod-cache:
    driver: local

  # Build cache
  go-build-cache:
    driver: local

  # Database persistence
  postgres-data:
    driver: local
```

---

## Environment Variables

```yaml
# .env file
ENV=development
LOG_LEVEL=debug
API_PORT=8080
DATABASE_URL=postgres://muster:muster_dev@postgres:5432/muster_test
CACHE_DIR=/tmp/muster-cache

# docker-compose.yaml
services:
  muster:
    env_file:
      - .env
    environment:
      # Override or add specific vars
      - VERSION=${VERSION:-dev}
      - BUILD_TIME=${BUILD_TIME:-unknown}
```

---

## Full Development Stack

```yaml
# docker-compose.full.yaml
version: '3.8'

services:
  muster:
    build: .
    depends_on:
      postgres:
        condition: service_healthy
      redis:
        condition: service_healthy
    environment:
      DATABASE_URL: postgres://muster:muster_dev@postgres:5432/muster
      REDIS_URL: redis:6379
    ports:
      - "8080:8080"

  postgres:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: muster
      POSTGRES_PASSWORD: muster_dev
      POSTGRES_DB: muster
    volumes:
      - postgres-data:/var/lib/postgresql/data

  redis:
    image: redis:7-alpine
    volumes:
      - redis-data:/data
    healthcheck:
      test: ["CMD", "redis-cli", "ping"]

volumes:
  postgres-data:
  redis-data:
```
