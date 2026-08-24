.PHONY: run build test vet fmt tidy clean

run:
	go run ./cmd/main

build:
	go build -o bin/server ./cmd/main

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -w .

tidy:
	go mod tidy

clean:
	rm -rf bin
