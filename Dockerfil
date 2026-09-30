FROM golang:1.27.1-alpine AS builder

WORKDIR /app

COPY go.mod go.sum* ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o /app/bin/api ./cmd/api


FROM alpine:3.22

WORKDIR /app

RUN addgroup -S app && adduser -S app -G app

COPY --from=builder /app/bin/api /app/api

USER app

EXPOSE 8080

CMD ["/app/api"]