.PHONY: build build-mcp build-scan build-ingest build-all test run clean frontend

frontend:
	cd frontend && npm run build

build: frontend
	go build -o bin/prd2wiki ./cmd/prd2wiki

build-mcp:
	go build -o bin/prd2wiki-mcp ./cmd/prd2wiki-mcp

build-scan:
	go build -o bin/prd2wiki-scan ./cmd/prd2wiki-scan

build-ingest:
	go build -o bin/prd2wiki-ingest ./cmd/prd2wiki-ingest

build-keygen:
	go build -o bin/prd2wiki-keygen ./cmd/prd2wiki-keygen

build-all: build build-mcp build-scan build-ingest build-keygen

test:
	go test ./... -v -count=1

run: build
	./bin/prd2wiki -config config/prd2wiki.yaml

clean:
	rm -rf bin/
