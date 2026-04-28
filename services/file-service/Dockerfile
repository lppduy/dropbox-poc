FROM golang:1.24-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /file-service ./services/file-service/cmd/api

FROM alpine:3.20
RUN apk --no-cache add ca-certificates
COPY --from=builder /file-service /file-service
EXPOSE 8081
ENTRYPOINT ["/file-service"]
