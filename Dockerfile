FROM golang:1.14-alpine

RUN apk add git
RUN go get -d -v github.com/mmcdole/gofeed

COPY . /go/src/app
WORKDIR /go/src/app
ARG release
RUN go build -o main -ldflags "-X main.Version=${release:-$(git describe --abbrev=0 --tags)-$(git rev-list -1 --abbrev-commit HEAD)}" .


FROM alpine:latest
WORKDIR /app
COPY --from=0 /go/src/app/main .
COPY config.json /app/config.json
ENTRYPOINT ["/app/main"]
CMD ["--config", "/app/config.json"]
