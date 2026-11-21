FROM golang:1.25-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/yingce-backend ./cmd/server && \
    CGO_ENABLED=0 go build -o /out/migrate-schema ./cmd/migrate-schema

FROM node:22.19-alpine
RUN apk add --no-cache ffmpeg ca-certificates
WORKDIR /app
COPY --from=builder /out/yingce-backend /usr/local/bin/yingce-backend
COPY --from=builder /out/migrate-schema /usr/local/bin/migrate-schema
EXPOSE 8080
CMD ["/usr/local/bin/yingce-backend"]
