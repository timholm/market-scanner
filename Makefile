BINARY := market-scanner
MODULE := github.com/timholm/market-scanner
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -ldflags "-s -w -X main.version=$(VERSION)"
IMAGE := ghcr.io/timholm/market-scanner:latest

.PHONY: build run test clean docker push deploy

build:
	CGO_ENABLED=1 go build $(LDFLAGS) -o bin/$(BINARY) .

run: build
	./bin/$(BINARY) scan --name "example-tool" --problem "example problem"

serve: build
	./bin/$(BINARY) serve

test:
	CGO_ENABLED=1 go test -v -race -count=1 ./...

clean:
	rm -rf bin/

lint:
	golangci-lint run ./...

docker:
	docker build -t $(IMAGE) .

docker-arm64:
	docker buildx build --platform linux/arm64 -t $(IMAGE) --push .

push: docker
	docker push $(IMAGE)

deploy:
	kubectl apply -f deploy/
