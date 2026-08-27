# ==============================================================================
# OmniCast Engine - Automated Build & Deployment Automation
# ==============================================================================

DOCKER_USER ?= meharab
IMAGE_NAME ?= omnicast_engine
TAG ?= latest

FULL_IMAGE = $(DOCKER_USER)/$(IMAGE_NAME):$(TAG)

.PHONY: all build push release test dev prod-up prod-down logs clean help

all: build

help: ## Show this help message
	@echo "OmniCast Engine Build & Deployment Commands:"
	@echo "  make build       - Build the All-In-One Docker image using Dockerfile.aio ($(FULL_IMAGE))"
	@echo "  make push        - Push $(FULL_IMAGE) to Docker Hub"
	@echo "  make release     - Build and push the All-In-One image automatically"
	@echo "  make run-aio     - Run the All-In-One container with a single docker run command"
	@echo "  make test        - Run all Go unit and integration tests"
	@echo "  make dev         - Start the local dev stack with docker-compose.yml"
	@echo "  make prod-up     - Start the production stack with docker-compose.production.yml"
	@echo "  make prod-down   - Stop the production stack"
	@echo "  make logs        - View container logs"
	@echo "  make clean       - Clean temporary binaries and Docker cache"

test: ## Run Go unit tests
	@echo "==> Running Go unit tests..."
	GOTOOLCHAIN=local go test -v ./...

build: ## Build the All-In-One Docker image locally using Dockerfile.aio
	@echo "==> Building All-In-One Docker image $(FULL_IMAGE) from Dockerfile.aio..."
	docker build -f Dockerfile.aio -t $(FULL_IMAGE) -t $(DOCKER_USER)/$(IMAGE_NAME):$$(git rev-parse --short HEAD 2>/dev/null || echo "latest") .

push: ## Push the Docker image to Docker Hub
	@echo "==> Pushing $(FULL_IMAGE) to Docker Hub..."
	docker push $(FULL_IMAGE)

release: build push ## Build and push the All-In-One Docker image to Docker Hub
	@echo "==> Successfully released $(FULL_IMAGE) to Docker Hub!"

run-aio: ## Run the monolithic container in standalone mode
	@echo "==> Starting All-In-One OmniCast container..."
	docker run -d --name omnicast_aio --restart unless-stopped \
		-p 80:80 -p 443:443 -p 443:443/udp \
		-p 3478:3478 -p 3478:3478/udp \
		-p 49152-49250:49152-49250/udp \
		-p 50000-50050:50000-50050/udp \
		-v $(PWD)/data:/app/data \
		-v $(PWD)/config:/app/config \
		-v caddy_data:/data \
		--env-file .env \
		$(FULL_IMAGE)

dev: ## Run local development environment
	@echo "==> Starting local development stack..."
	docker compose -f docker-compose.yml up --build -d

prod-up: ## Run production environment pulling from Docker Hub
	@echo "==> Starting production stack from Docker Hub..."
	docker compose -f docker-compose.production.yml pull
	docker compose -f docker-compose.production.yml up -d

prod-down: ## Stop production environment
	@echo "==> Stopping production stack..."
	docker compose -f docker-compose.production.yml down

logs: ## View real-time logs of the running services
	docker compose -f docker-compose.production.yml logs -f

clean: ## Remove local compiled binaries
	@echo "==> Cleaning up..."
	rm -f omnicast-engine media-server
