VERSION ?= 1.0.0
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: build test vet lint check clean

build:
	GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "$(LDFLAGS)" -o Setup.exe ./cmd/setup

test:
	go test ./...

vet:
	go vet ./...

lint: vet
	@command -v staticcheck >/dev/null 2>&1 || { echo "staticcheck not installed: go install honnef.co/go/tools/cmd/staticcheck@latest"; exit 1; }
	staticcheck ./...

check: test vet

clean:
	rm -f Setup.exe
