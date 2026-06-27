FROM golang:1.23-bookworm AS build

WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/authcli ./cmd/authcli

FROM debian:bookworm-slim

RUN apt-get update && \
    apt-get install -y --no-install-recommends tzdata && \
    rm -rf /var/lib/apt/lists/*

RUN useradd --create-home --uid 10001 authcli
WORKDIR /app
COPY --from=build /out/authcli /usr/local/bin/authcli
COPY migrations ./migrations
RUN mkdir -p /data && chown -R authcli:authcli /data /app

USER authcli
ENV DB_DSN=/data/auth.db
ENTRYPOINT ["authcli"]
