# STEP 1 build executable binary

#FROM golang:alpine as builder
FROM golang:1.20.3-alpine3.17 as builder

RUN apk update
RUN apk add git

# Create appuser on builder image
RUN adduser -D -g '' appuser

COPY ./cmd/ /tmp/app/cmd
COPY ./gateboard/ /tmp/app/gateboard
COPY ./metrics/ /tmp/app/metrics
COPY ./tracing/ /tmp/app/tracing
COPY go.* /tmp/app/
WORKDIR /tmp/app
RUN go mod tidy
RUN go env -w CGO_ENABLED=0
RUN go build -o /tmp/gateboard github.com/udhos/gateboard/cmd/gateboard

#
# STEP 2 build a small image from alpine
#
# curl: https://security.alpinelinux.org/srcpkg/curl
#
FROM alpine:3.17.3
COPY --from=builder /tmp/gateboard /bin/gateboard
RUN apk add curl=8.0.1-r0
RUN adduser -D -g '' user
USER user
ENTRYPOINT ["/bin/gateboard"]
