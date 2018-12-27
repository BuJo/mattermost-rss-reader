GIT_COMMIT=$(shell git rev-list -1 --abbrev-commit HEAD)
GIT_TAG=$(shell git describe --abbrev=0 --tags)
release=$(GIT_TAG)-$(GIT_COMMIT)

mattermost-rss-reader: *.go
	go build -ldflags "-X main.Version=${release}" .

.PHONY: fmt all clean
fmt: *.go
	gofmt -w -s $?
clean:
	rm -f mattermost-rss-reader
all: fmt mattermost-rss-reader
