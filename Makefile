VERSION := 0.1.0
BINARY := tokara
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: build test clean release

build:
	go build -ldflags "$(LDFLAGS)" -o $(BINARY) .

test:
	go test ./... -v

clean:
	rm -f $(BINARY)
	rm -rf dist/

release: clean
	mkdir -p dist
	GOOS=darwin GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o dist/$(BINARY)-darwin-arm64 .
	GOOS=darwin GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o dist/$(BINARY)-darwin-amd64 .
	GOOS=linux GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o dist/$(BINARY)-linux-arm64 .
	GOOS=linux GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o dist/$(BINARY)-linux-amd64 .
	GOOS=windows GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o dist/$(BINARY)-windows-amd64.exe .
