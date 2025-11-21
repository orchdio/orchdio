build: ## Build Docker image
	docker-compose build

run: ## Run in Docker (production: uses external DB/Redis from .env)
	docker-compose build
	ORCHDIO_ENV=production docker-compose up -d app

run-dev: ## Run in Docker (dev: uses Docker postgres/redis)
	ORCHDIO_ENV=dev docker-compose up -d

rebuild: ## Rebuild (production)
	docker-compose down
	ORCHDIO_ENV=production docker-compose up -d --build orchdio_app

rebuild-dev: ## Rebuild (dev)
	docker-compose down
	ORCHDIO_ENV=dev docker-compose up -d --build

stop: ## Stop Docker containers
	docker-compose down

clean: ## Stop and remove containers, volumes, and images
	docker-compose down -v --rmi all
