#!/bin/sh
set -e

# Load env file based on ORCHDIO_ENV
if [ "$ORCHDIO_ENV" = "dev" ] || [ "$ORCHDIO_ENV" = "development" ]; then
    [ -f .env.dev ] && set -a && . .env.dev && set +a

    # Override to use Docker services in dev mode
    export DATABASE_URL="postgresql://postgres:postgres@postgres:5432/orchdio"
    export REDISCLOUD_URL="redis://redis:6379"

    echo "Dev mode: Using Docker postgres and redis"
    until nc -z postgres 5432; do sleep 1; done
    until nc -z redis 6379; do sleep 1; done
else
    [ -f .env ] && set -a && . .env && set +a
    echo "Production mode: Using external DB/Redis from .env"
fi

echo "DATABASE_URL: $DATABASE_URL"
echo "REDISCLOUD_URL: $REDISCLOUD_URL"

exec "$@"
