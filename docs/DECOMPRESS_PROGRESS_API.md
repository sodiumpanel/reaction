# Compression & Decompression Progress API

This document describes the WebSocket events for tracking file compression and decompression progress in Sodium.

## Overview

When a decompression operation is initiated via the `POST /api/servers/:uuid/files/decompress` endpoint, the server emits progress events via WebSocket. This allows clients to display real-time progress to users.

## WebSocket Events

### `decompress progress`

Emitted periodically during decompression (approximately every 250ms when there's activity).

#### Payload

```json
{
    "file": "archive.tar.gz",
    "current_file": "path/to/current/file.txt",
    "total_files": 150,
    "processed_files": 45,
    "total_bytes": 104857600,
    "processed_bytes": 31457280,
    "progress": 0.30,
    "percent": 30
}
```

| Field | Type | Description |
|-------|------|-------------|
| `file` | string | Name of the archive being extracted |
| `current_file` | string | Path of the file currently being extracted |
| `total_files` | int64 | Total number of files in the archive |
| `processed_files` | int64 | Number of files already extracted |
| `total_bytes` | int64 | Total uncompressed size in bytes |
| `processed_bytes` | int64 | Bytes already extracted |
| `progress` | float64 | Progress as decimal (0.0 to 1.0) |
| `percent` | int | Progress as percentage (0 to 100) |

### `decompress completed`

Emitted when decompression finishes, whether successful or not.

#### Payload

```json
{
    "file": "archive.tar.gz",
    "total_files": 150,
    "total_bytes": 104857600,
    "success": true,
    "error": ""
}
```

| Field | Type | Description |
|-------|------|-------------|
| `file` | string | Name of the archive that was extracted |
| `total_files` | int64 | Total number of files that were extracted |
| `total_bytes` | int64 | Total bytes that were extracted |
| `success` | bool | Whether the operation completed successfully |
| `error` | string | Error message if `success` is false, empty otherwise |

## Usage Example

### JavaScript WebSocket Client

```javascript
// Connect to the server WebSocket
const ws = new WebSocket('wss://your-wings-server/api/servers/{uuid}/ws');

ws.onmessage = (event) => {
    const message = JSON.parse(event.data);
    
    switch (message.event) {
        case 'decompress progress':
            const progress = message.args[0];
            console.log(`Extracting: ${progress.current_file}`);
            console.log(`Progress: ${progress.percent}%`);
            console.log(`Files: ${progress.processed_files}/${progress.total_files}`);
            
            // Update progress bar
            updateProgressBar(progress.percent);
            break;
            
        case 'decompress completed':
            const result = message.args[0];
            if (result.success) {
                console.log(`Extraction complete! ${result.total_files} files extracted.`);
            } else {
                console.error(`Extraction failed: ${result.error}`);
            }
            break;
    }
};

// Trigger decompression via HTTP API
fetch('/api/servers/{uuid}/files/decompress', {
    method: 'POST',
    headers: {
        'Content-Type': 'application/json',
        'Authorization': 'Bearer your-token'
    },
    body: JSON.stringify({
        root: '/',
        file: 'archive.tar.gz'
    })
});
```

### React Hook Example

```typescript
import { useState, useEffect } from 'react';

interface DecompressProgress {
    file: string;
    current_file: string;
    total_files: number;
    processed_files: number;
    total_bytes: number;
    processed_bytes: number;
    progress: number;
    percent: number;
}

function useDecompressProgress(ws: WebSocket) {
    const [progress, setProgress] = useState<DecompressProgress | null>(null);
    const [isComplete, setIsComplete] = useState(false);
    const [error, setError] = useState<string | null>(null);

    useEffect(() => {
        const handleMessage = (event: MessageEvent) => {
            const message = JSON.parse(event.data);
            
            if (message.event === 'decompress progress') {
                setProgress(message.args[0]);
            } else if (message.event === 'decompress completed') {
                const result = message.args[0];
                setIsComplete(true);
                if (!result.success) {
                    setError(result.error);
                }
            }
        };

        ws.addEventListener('message', handleMessage);
        return () => ws.removeEventListener('message', handleMessage);
    }, [ws]);

    return { progress, isComplete, error };
}
```

## API Endpoint

### POST /api/servers/:uuid/files/decompress

Decompresses an archive file within the server's file system.

#### Request Body

```json
{
    "root": "/",
    "file": "archive.tar.gz"
}
```

| Field | Type | Description |
|-------|------|-------------|
| `root` | string | Directory path where the archive is located |
| `file` | string | Name of the archive file to decompress |

#### Response

- **204 No Content**: Decompression completed successfully
- **400 Bad Request**: Invalid archive format or file busy
- **500 Internal Server Error**: Decompression failed

#### Supported Archive Formats

- `.tar.gz` / `.tgz`
- `.tar.bz2` / `.tbz2`
- `.tar.xz` / `.txz`
- `.tar`
- `.zip`
- `.rar`
- `.7z`
- `.gz` (single file)
- `.bz2` (single file)
- `.xz` (single file)

## Implementation Notes

1. **Progress Interval**: Progress events are throttled to emit at most every 250ms to prevent flooding the WebSocket connection.

2. **Archive Analysis**: Before decompression begins, the archive is analyzed to determine total file count and size. This adds a small overhead but enables accurate progress reporting.

3. **Single-File Archives**: For single-file compressed files (`.gz`, `.bz2`, `.xz`), progress is reported based on bytes written rather than file count.

4. **Error Handling**: If decompression fails, a `decompress completed` event is still emitted with `success: false` and the error message.

---

# Compression Progress

## Overview

When a compression operation is initiated via the `POST /api/servers/:uuid/files/compress` endpoint, the server emits progress events via WebSocket.

## WebSocket Events

### `compress progress`

Emitted periodically during compression (approximately every 250ms when there's activity).

#### Payload

```json
{
    "total_files": 150,
    "processed_files": 45,
    "total_bytes": 104857600,
    "processed_bytes": 31457280,
    "progress": 0.30,
    "percent": 30
}
```

| Field | Type | Description |
|-------|------|-------------|
| `total_files` | int64 | Total number of files to compress |
| `processed_files` | int64 | Number of files already compressed |
| `total_bytes` | int64 | Total size of source files in bytes |
| `processed_bytes` | int64 | Bytes already processed |
| `progress` | float64 | Progress as decimal (0.0 to 1.0) |
| `percent` | int | Progress as percentage (0 to 100) |

### `compress completed`

Emitted when compression finishes, whether successful or not.

#### Payload

```json
{
    "total_files": 150,
    "total_bytes": 104857600,
    "archive_size": 52428800,
    "success": true,
    "error": ""
}
```

| Field | Type | Description |
|-------|------|-------------|
| `total_files` | int64 | Total number of files that were compressed |
| `total_bytes` | int64 | Total bytes of source files |
| `archive_size` | int64 | Size of the resulting archive in bytes |
| `success` | bool | Whether the operation completed successfully |
| `error` | string | Error message if `success` is false, empty otherwise |

## API Endpoint

### POST /api/servers/:uuid/files/compress

Compresses files within the server's file system.

#### Request Body

```json
{
    "root": "/",
    "files": ["folder1", "file.txt", "folder2/subfolder"]
}
```

| Field | Type | Description |
|-------|------|-------------|
| `root` | string | Directory path where the files are located |
| `files` | array | List of files and directories to compress |

#### Response

```json
{
    "name": "archive-2026-01-28T123456Z.tar.gz",
    "created": "2026-01-28T12:34:56Z",
    "modified": "2026-01-28T12:34:56Z",
    "size": 52428800,
    "directory": false,
    "file": true,
    "mime": "application/tar+gzip"
}
```

## JavaScript Example

```javascript
ws.onmessage = (event) => {
    const message = JSON.parse(event.data);
    
    switch (message.event) {
        case 'compress progress':
            const progress = message.args[0];
            console.log(`Compressing: ${progress.percent}%`);
            console.log(`Files: ${progress.processed_files}/${progress.total_files}`);
            updateProgressBar(progress.percent);
            break;
            
        case 'compress completed':
            const result = message.args[0];
            if (result.success) {
                console.log(`Compression complete! Archive size: ${result.archive_size} bytes`);
            } else {
                console.error(`Compression failed: ${result.error}`);
            }
            break;
    }
};
```

---

## Related Files

- `server/filesystem/decompress_progress.go` - Decompression progress tracking
- `server/filesystem/compress_progress.go` - Compression progress tracking
- `server/filesystem/compress.go` - Compression/decompression logic
- `server/filesystem/archive.go` - Archive creation logic
- `server/events.go` - Event constants
- `router/router_server_files.go` - HTTP endpoint handlers
