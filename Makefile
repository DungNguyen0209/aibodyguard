BINARY  := aibodyguard
CMD     := ./cmd/aibodyguard

.PHONY: build test lint clean

build:
	go build -o $(BINARY) $(CMD)

test:
	go test ./...

lint:
	staticcheck ./...

clean:
	rm -f $(BINARY)
