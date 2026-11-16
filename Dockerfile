# Builder stage
ARG GO_VERSION=1.27.4
FROM golang:${GO_VERSION}-alpine3.24 AS builder
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOTOOLCHAIN=local go build -trimpath -o bot ./cmd/bot

# Runtime stage
FROM alpine:3.24.4
RUN apk upgrade --no-cache && apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=builder /build/bot .
COPY --from=builder /build/config-simple.json .
EXPOSE 25
STOPSIGNAL SIGTERM
CMD ["./bot"]
