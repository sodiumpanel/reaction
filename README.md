# Sodium Reaction

Server daemon for game server management. Provides an HTTP API for managing server instances, backups, file management, and real-time console access.

Fork of [Pterodactyl Wings](https://github.com/pterodactyl/wings), optimized for high-density deployments (10,000+ users).

## Key Improvements over Pterodactyl Wings

- **SQLite WAL mode** — concurrent database access without blocking
- **O(1) server lookups** — map-based server manager instead of linear scans
- **Graceful shutdown** — clean signal handling with state persistence
- **Health endpoint** — `GET /api/health` for monitoring and load balancers
- **Recursive directory copy** — copy entire directories via the file manager

## Installation

```bash
sudo ./wings --config /etc/sodium/config.yml
```

## Building

```bash
# Single platform
go build -ldflags="-s -w" -o wings .

# Cross-compile (amd64 + arm64)
make build
```

## Health Check

```bash
curl http://localhost:8080/api/health
# {"healthy":true,"docker":true,"server_count":5,"version":"1.1.0"}
```

## Features

- HTTP API for server control
- Built-in SFTP server
- Docker container isolation
- Backup support (local and S3)
- Real-time WebSocket events

## License

MIT License — Copyright 2025 zt3xdv (tsumugi_dev)
