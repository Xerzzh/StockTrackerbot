BINARY := stocktrackerbot

.PHONY: build run test vet fmt clean

build:
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o $(BINARY) .

run:
	go run .

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -w *.go

clean:
	rm -f $(BINARY)
	rm -rf config
