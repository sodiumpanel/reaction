# Sodium

Server daemon for game server management. Provides an HTTP API for managing server instances, backups, file management, and real-time console access.

## Installation

```bash
sudo ./wings --config /etc/sodium/config.yml
```

## Building

```bash
go build -ldflags="-s -w" -o wings .
```

## Features

- HTTP API for server control
- Built-in SFTP server
- Docker container isolation
- Backup support (local and S3)
- Real-time WebSocket events

## License

MIT License - Copyright 2025 zt3xdv (tsumugi_dev)

Fork of [Pterodactyl Wings](https://github.com/pterodactyl/wings).
