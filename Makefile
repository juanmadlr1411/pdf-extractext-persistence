.PHONY: build up down test

down:
	docker compose down

build:
	docker compose build

up:
	docker compose up -d

test:
	go test ./...