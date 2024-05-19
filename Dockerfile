FROM golang:1.22.3-alpine AS builder

WORKDIR /go/src/github.com/quangnguyen/registrator/

COPY . .

RUN go get -v -t .
RUN go build -o bin/registrator -ldflags="-X main.version=$VERSION" .

FROM alpine:3.19

RUN apk add --no-cache ca-certificates

WORKDIR /app

COPY --from=builder /go/src/github.com/quangnguyen/registrator/bin/registrator /bin
COPY --from=builder /go/src/github.com/quangnguyen/registrator/entrypoint.sh ./

RUN chmod +x /app/entrypoint.sh

ENTRYPOINT ["/app/entrypoint.sh"]