FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY . .
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -o pdf-processor ./cmd/api

FROM alpine:latest
WORKDIR /app
COPY --from=builder /app/pdf-processor .
COPY .env.example .env
EXPOSE 8080
CMD ["./pdf-processor"]

