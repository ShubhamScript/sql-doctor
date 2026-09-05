BINARY_NAME=sql-doctor
VERSION?=1.0.0

.PHONY: all build test clean run docker-up docker-down lint

all: build test

build:
	go build -ldflags="-w -s" -o $(BINARY_NAME) ./cmd/sql-doctor

test:
	go test -v ./...

lint:
	go vet ./...

clean:
	rm -f $(BINARY_NAME) $(BINARY_NAME).exe

docker-up:
	docker compose up -d

docker-down:
	docker compose down

run: build
	./$(BINARY_NAME) --help
