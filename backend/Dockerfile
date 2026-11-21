FROM golang:1.25-alpine AS builder
RUN apk add --no-cache gcc musl-dev
WORKDIR /src/backend
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=1 go build -o /out/yingce-backend ./cmd/server && \
    CGO_ENABLED=1 go build -o /out/migrate-schema ./cmd/migrate-schema

FROM node:22-alpine
RUN apk add --no-cache ffmpeg ca-certificates
WORKDIR /app
COPY --from=builder /out/yingce-backend /usr/local/bin/yingce-backend
COPY --from=builder /out/migrate-schema /usr/local/bin/migrate-schema
COPY plugin-packages/mock-echo/manifest.json /app/plugin-packages/mock-echo/manifest.json
EXPOSE 8080
CMD ["/usr/local/bin/yingce-backend"]
