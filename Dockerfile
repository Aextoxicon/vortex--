# Multi-stage build
FROM golang:1.26-alpine AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG VERSION=dev
ARG COMMIT=none
RUN CGO_ENABLED=0 go build -v -ldflags="-s -w -X main.Version=${VERSION} -X main.Commit=${COMMIT}" -o vortex .

# Runtime stage
FROM alpine:3.21

RUN apk add --no-cache ca-certificates libpq \
    && addgroup -S vortex && adduser -S vortex -G vortex

WORKDIR /app

COPY --from=builder /build/vortex .

RUN chown -R vortex:vortex /app

USER vortex

EXPOSE 9178

ENTRYPOINT ["/app/vortex"]
