# Builder stage
FROM golang:latest AS builder
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o bot ./cmd/bot

# Runtime stage
FROM alpine:latest
WORKDIR /app
COPY --from=builder /build/bot .
COPY --from=builder /build/config-simple.json .
EXPOSE 25
CMD ["./bot"]
