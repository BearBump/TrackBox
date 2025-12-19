.PHONY: up down logs build test cover generate

up:
	docker compose up -d --build

down:
	docker compose down

logs:
	docker compose logs -f --tail=200

build:
	docker compose build track-api track-worker

test:
	go test ./...

cover:
	go test -cover ./...

generate:
	@./scripts/generate.sh