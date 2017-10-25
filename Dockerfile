FROM golang:onbuild
COPY . /go/src/app
WORKDIR /go/src/app
RUN go build -o main .
CMD ["/go/src/app/main", "--config", "/go/src/app/config.json"]
