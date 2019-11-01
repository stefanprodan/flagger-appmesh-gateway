FROM golang:1.13 as builder

RUN mkdir -p /kxds/

WORKDIR /kxds

COPY . .

RUN go mod download

RUN go test -v -race ./...

RUN CGO_ENABLED=0 GOOS=linux go build -a -o bin/kxds cmd/kxds/*

FROM alpine:3.10

RUN addgroup -S app \
    && adduser -S -g app app \
    && apk --no-cache add \
    curl openssl netcat-openbsd

WORKDIR /home/app

COPY --from=builder /kxds/bin/kxds .
RUN chown -R app:app ./

USER app

CMD ["./kxds"]
