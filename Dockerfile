FROM golang:1.25-alpine AS builder
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o prd2wiki ./cmd/prd2wiki

FROM alpine:3.21
RUN apk add --no-cache ca-certificates git
COPY --from=builder /build/prd2wiki /usr/local/bin/prd2wiki
COPY config/prd2wiki-docker.yaml /etc/prd2wiki/prd2wiki.yaml
EXPOSE 8080
VOLUME /data
CMD ["prd2wiki", "-config", "/etc/prd2wiki/prd2wiki.yaml"]
