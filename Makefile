.PHONY: build build-mcp build-all test run clean frontend

frontend:
	cd frontend && npm run build

build: frontend
	go build -o bin/prd2wiki ./cmd/prd2wiki

build-mcp:
	go build -o bin/prd2wiki-mcp ./cmd/prd2wiki-mcp

build-all: build build-mcp

test:
	go test ./... -v -count=1

run: build
	./bin/prd2wiki -config config/prd2wiki.yaml

clean:
	rm -rf bin/
