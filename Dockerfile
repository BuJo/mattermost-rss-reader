FROM golang:1.11-alpine
COPY . /go/src/app
WORKDIR /go/src/app
RUN apk add git
RUN go get -d -v github.com/mmcdole/gofeed
RUN go build -o main .

FROM alpine:latest
WORKDIR /app
COPY --from=0 /go/src/app/main .
COPY config.json /app/config.json
CMD ["/app/main", "--config", "/app/config.json"]
