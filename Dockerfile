#
# build stage
#
FROM golang:1.21-alpine AS builder

RUN apk add git

COPY . /build
WORKDIR /build
ARG release
RUN CGO_ENABLED=0 go build -o main -ldflags "-s -w -extldflags '-static' -X main.Version=${release:-$(git describe --abbrev=0 --tags)-$(git rev-list -1 --abbrev-commit HEAD)}" .

#
# runtime image
#
FROM scratch AS runtime

WORKDIR /app

COPY --from=builder /build/main .
COPY config.json ./config.json
COPY --from=alpine:latest /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

EXPOSE 9090

ENTRYPOINT ["/app/main"]
CMD ["--config", "/app/config.json"]
