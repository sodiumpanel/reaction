# Changelog

## v1.1.0

### Performance & Scalability
- **SQLite WAL mode**: Changed from `journal_mode=MEMORY` to `WAL` and `synchronous=OFF` to `NORMAL`. Enables concurrent reads during writes and prevents data loss on crash. Max open connections scaled to `runtime.NumCPU()`.
- **O(1) server lookups**: Server manager refactored from `[]*Server` (linear scan) to `map[string]*Server`. Every HTTP request no longer iterates all servers to find a match.

### Reliability
- **Graceful shutdown**: Added signal handling (SIGINT/SIGTERM) with `http.Server.Shutdown()` and a 30-second timeout. Server states are persisted and contexts are canceled cleanly before exit.

### New Features
- **Health endpoint**: `GET /api/health` (no auth required) returns Docker connectivity status, server count, and daemon version.

### Other
- **Directory copy support**: `Copy()` now supports recursive directory copying, not just regular files.

## v1.0.0

Initial release — fork of Pterodactyl Wings.
