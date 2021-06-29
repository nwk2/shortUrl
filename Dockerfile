FROM golang:1.15

RUN mkdir -p /app

ADD . /app

WORKDIR /app

RUN go build .

CMD ["./shortUrl"]