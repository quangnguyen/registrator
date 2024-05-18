FROM golang:1.22.3-alpine AS builder

WORKDIR /go/src/github.com/quangnguyen/registrator/

COPY . .

RUN go get -v -t .
RUN go build \
        -a -installsuffix cgo \
		-ldflags "-X main.Version=$(cat VERSION)" \
		-o bin/registrator


FROM alpine:3.19

ENV APP_HOME /go/src/github.com/quangnguyen/registrator
RUN apk add --no-cache ca-certificates

WORKDIR $APP_HOME
COPY --from=builder /go/src/github.com/quangnguyen/registrator/bin/registrator .

ENTRYPOINT ["/bin/registrator"]
