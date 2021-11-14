#
# build stage
#
FROM golang:1.17-alpine AS builder

RUN apk add git
RUN go get -d -v github.com/mmcdole/gofeed

COPY . /go/src/app
WORKDIR /go/src/app
ARG release
RUN go build -o main -ldflags "-s -w -X main.Version=${release:-$(git describe --abbrev=0 --tags)-$(git rev-list -1 --abbrev-commit HEAD)}" .

#
# runtime image
#
FROM alpine:latest AS runtime

RUN apk add --no-cache ca-certificates

WORKDIR /app

COPY --from=builder /go/src/app/main .
COPY config.json ./config.json

EXPOSE 9090

ENTRYPOINT ["/app/main"]
CMD ["--config", "/app/config.json"]
