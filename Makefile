TARGET_BIN = netlify-cms-oauth-provider
TARGET_ARCH = amd64
SOURCE_MAIN = main.go
LDFLAGS = -s -w
GOPATH = $(CURDIR)
export GOPATH

all: build

clean:
	rm -rf src bin

build: get build-darwin build-linux build-windows

get:
	go get -d -v

build-darwin:
	CGO_ENABLED=0 GOOS=darwin GOARCH=$(TARGET_ARCH) go build -ldflags "$(LDFLAGS)" -o bin/$(TARGET_BIN)_darwin-amd64 $(SOURCE_MAIN)

build-linux:
	CGO_ENABLED=0 GOOS=linux GOARCH=$(TARGET_ARCH) go build -ldflags "$(LDFLAGS)" -o bin/$(TARGET_BIN)_linux-amd64 $(SOURCE_MAIN)

build-windows:
	CGO_ENABLED=0 GOOS=windows GOARCH=$(TARGET_ARCH) go build -ldflags "$(LDFLAGS)" -o bin/$(TARGET_BIN)_windows-amd64.exe $(SOURCE_MAIN)

start:
	go run $(SOURCE_MAIN)

test:
	curl -X POST http://localhost:3000/callback/deploy --header "X-Hub-Signature: sha1=blank" --header "X-GitHub-Event: test" --header "X-GitHub-Delivery: uuid" -d "{ json: body, kinda: true }"
