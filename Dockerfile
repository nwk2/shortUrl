FROM golang:1.23

RUN mkdir -p /app

ADD . /app

WORKDIR /app

RUN go build -o shortUrl ./cmd/shorturl

CMD ["./shortUrl"]