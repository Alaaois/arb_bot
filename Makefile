.PHONY: test build run

test:
	go test ./...

build:
	go build -o bin/arb-bot ./cmd/arb-bot

run:
	go run ./cmd/arb-bot -config configs/local.yaml

docker-up:
	docker compose up -d --build

docker-down:
	docker compose down
