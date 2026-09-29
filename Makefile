.PHONY: build run fmt vet lint test e2e check clean

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

# End-to-end tests drive the real binary in tmux (needs tmux; ~30s).
e2e:
	go test -tags e2e -count=1 -timeout 3m ./e2e/

# Run everything CI runs, in order. Run before pushing.
check:
	@test -z "$$(gofmt -l .)" || { echo "gofmt needed on:"; gofmt -l .; exit 1; }
	go vet ./...
	golangci-lint run
	go test ./...
	go build -o /dev/null .

clean:
	rm -f track
