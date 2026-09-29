.PHONY: build run fmt vet lint test check clean

build:
	go build -o track .

run:
	go run .

fmt:
	gofmt -l -w .

vet:
	go vet ./...

lint:
	golangci-lint run

test:
	go test ./...

# Run everything CI runs, in order. Run before pushing.
check:
	@test -z "$$(gofmt -l .)" || { echo "gofmt needed on:"; gofmt -l .; exit 1; }
	go vet ./...
	golangci-lint run
	go test ./...
	go build -o /dev/null .

clean:
	rm -f track
