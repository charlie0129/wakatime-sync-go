# Build frontend
FROM node:22-alpine AS frontend-builder
WORKDIR /app/web
COPY web/package*.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

# Build backend
FROM golang:1.25-alpine AS backend-builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o wakatime-sync .

# Final image
FROM alpine:3.23
RUN apk --no-cache add ca-certificates tzdata
WORKDIR /app

COPY --from=backend-builder /app/wakatime-sync .
COPY --from=frontend-builder /app/web/dist ./web/dist
COPY config.example.yaml ./config.yaml

EXPOSE 3040
VOLUME ["/app/data"]

ENV DATABASE_PATH=/app/data/wakatime.db

CMD ["./wakatime-sync", "-config", "/app/config.yaml"]
