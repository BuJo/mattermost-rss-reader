GIT_COMMIT := $(shell git rev-list -1 --abbrev-commit HEAD)
GIT_TAG := $(shell git describe --abbrev=0 --tags)
release ?= $(GIT_TAG)-$(GIT_COMMIT)

.PHONY: fmt all clean check test
all: check mattermost-rss-reader

mattermost-rss-reader: *.go
	@go build -ldflags "-X main.Version=${release}" .

fmt: *.go
	@gofmt -l -w -s $?

test: *.go
	@go test

check: *.go
	@test -z $(shell gofmt -l $? | tee /dev/stderr) || echo "[WARN] Fix formatting issues with 'make fmt'"
	@which staticcheck >/dev/null 2>&1 && staticcheck $? || true
	@go vet $?

clean:
	@rm -f mattermost-rss-reader
