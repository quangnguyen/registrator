FROM golang:1.22.3-alpine AS builder

WORKDIR /go/src/github.com/quangnguyen/registrator/

COPY . .

RUN go get -v -t .
RUN go build -o bin/registrator -ldflags="-X main.version=$VERSION -v -s" .

FROM gcr.io/distroless/static-debian12

WORKDIR /app

COPY --from=builder /go/src/github.com/quangnguyen/registrator/bin/registrator /bin

ENTRYPOINT ["/bin/registrator"]