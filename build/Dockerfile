FROM golang:1.25-alpine AS builder

WORKDIR /app

RUN apk add \
  g++ \
  git \
  libcap-setcap \
  make


RUN git clone https://github.com/coredns/coredns

WORKDIR /app/coredns

COPY ./build/plugin.cfg .

RUN go get github.com/marcobox/coredns-redis

RUN go generate && make

RUN setcap cap_net_bind_service=+ep coredns

FROM alpine:latest AS prod-stage

# Set working directory in the container
WORKDIR /

# Copy the built binary from the builder stage
COPY ./build/Corefile* .
COPY ./build/entrypoint.sh .
COPY --from=builder /app/coredns/coredns .

ENV \
  COREDNS_REDIS_PREFIX="hostsync/coredns:" \
  COREDNS_REDIS_DNS_TTL=60 \
  COREDNS_REDIS_URL="localhost:6379"

EXPOSE 53 53/udp

ENTRYPOINT [ "/entrypoint.sh" ]
