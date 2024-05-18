FROM golang:1.22.3-alpine AS builder

WORKDIR /go/src/github.com/quangnguyen/registrator/

COPY . .

RUN go get -v -t .
RUN go build -o bin/registrator .

FROM alpine:3.19

RUN apk add --no-cache ca-certificates
COPY --from=builder /go/src/github.com/quangnguyen/registrator/bin/registrator /bin

ENTRYPOINT ["/bin/registrator"]