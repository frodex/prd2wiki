.PHONY: build test run clean

build:
	go build -o bin/prd2wiki ./cmd/prd2wiki

test:
	go test ./... -v -count=1

run: build
	./bin/prd2wiki -config config/prd2wiki.yaml

clean:
	rm -rf bin/
