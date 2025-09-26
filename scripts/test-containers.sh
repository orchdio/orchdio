#!/bin/bash

# Test Containers Management Script for Orchdio
# This script helps manage testcontainer lifecycle

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Default values
FORCE_CLEANUP=false
SHOW_LOGS=false
VERBOSE=false

usage() {
    cat << EOF
Usage: $0 [COMMAND] [OPTIONS]

Commands:
    run         Run tests with container reuse (default)
    clean       Clean up test containers completely
    status      Show status of test containers
    logs        Show logs from test containers
    reset       Reset all test containers (clean + run)

Options:
    -f, --force         Force cleanup of containers
    -v, --verbose       Verbose output
    -l, --logs          Show container logs
    -h, --help          Show this help message

Examples:
    $0 run              # Run tests with container reuse
    $0 clean            # Clean up all test containers
    $0 status           # Show container status
    $0 reset            # Reset and recreate all containers
    $0 run -v           # Run tests with verbose output

EOF
}

log() {
    if [[ $VERBOSE == true ]]; then
        echo -e "${BLUE}[INFO]${NC} $1"
    fi
}

error() {
    echo -e "${RED}[ERROR]${NC} $1" >&2
}

success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

# Check if Docker/Podman is available
check_container_runtime() {
    if command -v docker &> /dev/null; then
        CONTAINER_CMD="docker"
    elif command -v podman &> /dev/null; then
        CONTAINER_CMD="podman"
    else
        error "Neither Docker nor Podman found. Please install one of them."
        exit 1
    fi
    log "Using container runtime: $CONTAINER_CMD"
}

# Show status of test containers
show_status() {
    echo -e "${BLUE}=== Test Container Status ===${NC}"

    # Look for containers with orchdio test labels
    local postgres_containers
    local redis_containers

    postgres_containers=$($CONTAINER_CMD ps -a --filter "label=orchdio.test.postgres=true" --format "table {{.Names}}\t{{.Status}}\t{{.Ports}}")
    redis_containers=$($CONTAINER_CMD ps -a --filter "label=orchdio.test.redis=true" --format "table {{.Names}}\t{{.Status}}\t{{.Ports}}")

    if [[ -n "$postgres_containers" && "$postgres_containers" != "NAMES	STATUS	PORTS" ]]; then
        echo -e "\n${GREEN}PostgreSQL Test Containers:${NC}"
        echo "$postgres_containers"
    else
        warn "No PostgreSQL test containers found"
    fi

    if [[ -n "$redis_containers" && "$redis_containers" != "NAMES	STATUS	PORTS" ]]; then
        echo -e "\n${GREEN}Redis Test Containers:${NC}"
        echo "$redis_containers"
    else
        warn "No Redis test containers found"
    fi

    # Show networks
    local test_networks
    test_networks=$($CONTAINER_CMD network ls --filter "label=orchdio.test.network=true" --format "table {{.Name}}\t{{.Driver}}")

    if [[ -n "$test_networks" && "$test_networks" != "NAME	DRIVER" ]]; then
        echo -e "\n${GREEN}Test Networks:${NC}"
        echo "$test_networks"
    else
        warn "No test networks found"
    fi
}

# Show logs from test containers
show_logs() {
    echo -e "${BLUE}=== Test Container Logs ===${NC}"

    # Get container IDs
    local postgres_ids
    local redis_ids

    postgres_ids=$($CONTAINER_CMD ps -q --filter "label=orchdio.test.postgres=true")
    redis_ids=$($CONTAINER_CMD ps -q --filter "label=orchdio.test.redis=true")

    if [[ -n "$postgres_ids" ]]; then
        echo -e "\n${GREEN}PostgreSQL Logs:${NC}"
        for id in $postgres_ids; do
            echo -e "${YELLOW}Container: $id${NC}"
            $CONTAINER_CMD logs --tail 50 "$id"
            echo ""
        done
    fi

    if [[ -n "$redis_ids" ]]; then
        echo -e "\n${GREEN}Redis Logs:${NC}"
        for id in $redis_ids; do
            echo -e "${YELLOW}Container: $id${NC}"
            $CONTAINER_CMD logs --tail 50 "$id"
            echo ""
        done
    fi
}

# Clean up test containers
cleanup_containers() {
    echo -e "${BLUE}=== Cleaning Up Test Containers ===${NC}"

    # Stop and remove containers
    local postgres_ids
    local redis_ids

    postgres_ids=$($CONTAINER_CMD ps -aq --filter "label=orchdio.test.postgres=true")
    redis_ids=$($CONTAINER_CMD ps -aq --filter "label=orchdio.test.redis=true")

    if [[ -n "$postgres_ids" ]]; then
        log "Stopping PostgreSQL containers: $postgres_ids"
        $CONTAINER_CMD stop $postgres_ids 2>/dev/null || true
        log "Removing PostgreSQL containers: $postgres_ids"
        $CONTAINER_CMD rm $postgres_ids 2>/dev/null || true
        success "PostgreSQL containers cleaned up"
    fi

    if [[ -n "$redis_ids" ]]; then
        log "Stopping Redis containers: $redis_ids"
        $CONTAINER_CMD stop $redis_ids 2>/dev/null || true
        log "Removing Redis containers: $redis_ids"
        $CONTAINER_CMD rm $redis_ids 2>/dev/null || true
        success "Redis containers cleaned up"
    fi

    # Clean up networks
    local network_ids
    network_ids=$($CONTAINER_CMD network ls -q --filter "label=orchdio.test.network=true")

    if [[ -n "$network_ids" ]]; then
        log "Removing test networks: $network_ids"
        $CONTAINER_CMD network rm $network_ids 2>/dev/null || true
        success "Test networks cleaned up"
    fi

    # Clean up volumes (be careful with this)
    if [[ $FORCE_CLEANUP == true ]]; then
        warn "Force cleanup enabled - removing test volumes"
        local volume_ids
        volume_ids=$($CONTAINER_CMD volume ls -q --filter "label=orchdio.test=true" 2>/dev/null || true)
        if [[ -n "$volume_ids" ]]; then
            $CONTAINER_CMD volume rm $volume_ids 2>/dev/null || true
            success "Test volumes cleaned up"
        fi
    fi
}

# Run tests
run_tests() {
    echo -e "${BLUE}=== Running Tests with Container Reuse ===${NC}"

    # Change to project root
    cd "$(dirname "$0")/.."

    # Set environment variables for container reuse
    export TESTCONTAINERS_REUSE_ENABLE=true
    export TESTCONTAINERS_RYUK_DISABLED=true

    local test_flags=""
    if [[ $FORCE_CLEANUP == true ]]; then
        test_flags="$test_flags -force-cleanup"
    fi

    if [[ $VERBOSE == true ]]; then
        test_flags="$test_flags -v"
    fi

    log "Running: go test -tags=integration $test_flags ./..."

    if go test -tags=integration $test_flags ./...; then
        success "Tests completed successfully"
    else
        error "Tests failed"
        if [[ $SHOW_LOGS == true ]]; then
            show_logs
        fi
        return 1
    fi
}

# Parse command line arguments
parse_args() {
    while [[ $# -gt 0 ]]; do
        case $1 in
            -f|--force)
                FORCE_CLEANUP=true
                shift
                ;;
            -v|--verbose)
                VERBOSE=true
                shift
                ;;
            -l|--logs)
                SHOW_LOGS=true
                shift
                ;;
            -h|--help)
                usage
                exit 0
                ;;
            *)
                break
                ;;
        esac
    done
}

# Main function
main() {
    local command=${1:-run}
    shift || true

    parse_args "$@"
    check_container_runtime

    case $command in
        run)
            run_tests
            ;;
        clean)
            cleanup_containers
            ;;
        status)
            show_status
            ;;
        logs)
            show_logs
            ;;
        reset)
            cleanup_containers
            sleep 2
            run_tests
            ;;
        *)
            error "Unknown command: $command"
            usage
            exit 1
            ;;
    esac
}

# Run main function with all arguments
main "$@"
