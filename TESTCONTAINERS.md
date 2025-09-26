# Testcontainers Setup and Container Reuse

This document explains how testcontainers are configured in this project to reuse containers across test runs, improving test performance and reducing resource usage.

## Overview

The project uses testcontainers-go with container reuse enabled to avoid creating new containers on every test run. This significantly speeds up test execution and reduces Docker resource consumption.

## Configuration

### Environment Variables

The following environment variables are automatically set to enable container reuse:

```bash
TESTCONTAINERS_REUSE_ENABLE=true
TESTCONTAINERS_RYUK_DISABLED=true
TESTCONTAINERS_DOCKER_SOCKET_OVERRIDE=/var/run/docker.sock
```

### Container Labels

Containers are created with specific labels to enable identification and reuse:

- **PostgreSQL**: `orchdio.test.postgres=true`
- **Redis**: `orchdio.test.redis=true`
- **Network**: `orchdio.test.network=true`

### Default Container Images

- **PostgreSQL**: `postgres:15`
- **Redis**: `redis:7-alpine`

## Usage

### Running Tests

#### Standard Test Run (with container reuse)
```bash
go test -tags=integration ./...
```

#### Using the Management Script
```bash
# Run tests with container reuse
./scripts/test-containers.sh run

# Run tests with verbose output
./scripts/test-containers.sh run -v

# Check container status
./scripts/test-containers.sh status

# View container logs
./scripts/test-containers.sh logs

# Clean up all containers
./scripts/test-containers.sh clean

# Reset containers (clean + recreate)
./scripts/test-containers.sh reset
```

#### Force Cleanup After Tests
```bash
go test -tags=integration -force-cleanup ./...
# or
./scripts/test-containers.sh run -f
```

### Test Infrastructure

The `TestInfrastructure` struct provides:

- **Singleton Pattern**: Only one set of containers per test process
- **Connection Health Checks**: Automatically verifies container health before reuse
- **Graceful Cleanup**: Cleans test data without terminating containers
- **Connection Pooling**: Optimized database and Redis connection settings

```go
// Example usage in tests
func TestSomething(t *testing.T) {
    infra := testutils.GetOrCreateInfrastructure(t)
    
    // Use