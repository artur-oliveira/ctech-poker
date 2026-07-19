FROM golang:1.26-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -o /app-bin ./cmd/server

FROM gcr.io/distroless/static-debian12

COPY --from=builder /app-bin /app

EXPOSE 8003

ENTRYPOINT ["/app"]
