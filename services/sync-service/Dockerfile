FROM golang:1.24-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /sync-service ./services/sync-service/cmd/api

FROM alpine:3.20
RUN apk --no-cache add ca-certificates
COPY --from=builder /sync-service /sync-service
EXPOSE 8082
ENTRYPOINT ["/sync-service"]
