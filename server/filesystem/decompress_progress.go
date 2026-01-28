package filesystem

import (
	"sync"
	"sync/atomic"
	"time"
)

// DecompressProgress tracks the progress of a decompression operation.
// It provides real-time progress updates that can be consumed via WebSocket events.
//
// # WebSocket Events
//
// The decompression progress is reported via two WebSocket events:
//
// ## "decompress progress" event
//
// Sent periodically during decompression with the following JSON payload:
//
//	{
//	    "file": "archive.tar.gz",           // Name of the archive being extracted
//	    "current_file": "path/to/file.txt", // File currently being extracted
//	    "total_files": 150,                 // Total number of files in the archive
//	    "processed_files": 45,              // Number of files already extracted
//	    "total_bytes": 104857600,           // Total uncompressed size in bytes
//	    "processed_bytes": 31457280,        // Bytes already extracted
//	    "progress": 0.30,                   // Progress as decimal (0.0 to 1.0)
//	    "percent": 30                       // Progress as percentage (0 to 100)
//	}
//
// ## "decompress completed" event
//
// Sent when decompression finishes (success or failure):
//
//	{
//	    "file": "archive.tar.gz",
//	    "total_files": 150,
//	    "total_bytes": 104857600,
//	    "success": true,
//	    "error": ""                         // Error message if success is false
//	}
//
// # Usage Example
//
// Progress events are automatically emitted when using DecompressFileWithProgress:
//
//	err := server.Filesystem().DecompressFileWithProgress(ctx, "/", "archive.zip", func(p DecompressProgressData) {
//	    server.Events().Publish(server.DecompressProgressEvent, p)
//	})
type DecompressProgress struct {
	mu sync.RWMutex

	// Archive file name being extracted
	archiveFile string

	// Current file being extracted
	currentFile string

	// Total number of files in the archive
	totalFiles int64

	// Number of files already processed
	processedFiles atomic.Int64

	// Total uncompressed size in bytes
	totalBytes int64

	// Bytes already processed
	processedBytes atomic.Int64

	// Start time of the operation
	startTime time.Time

	// Callback function for progress updates
	callback func(DecompressProgressData)

	// Minimum interval between progress updates
	updateInterval time.Duration

	// Last time progress was reported
	lastUpdate time.Time
}

// DecompressProgressData represents a snapshot of decompression progress.
type DecompressProgressData struct {
	File           string  `json:"file"`
	CurrentFile    string  `json:"current_file"`
	TotalFiles     int64   `json:"total_files"`
	ProcessedFiles int64   `json:"processed_files"`
	TotalBytes     int64   `json:"total_bytes"`
	ProcessedBytes int64   `json:"processed_bytes"`
	Progress       float64 `json:"progress"`
	Percent        int     `json:"percent"`
}

// DecompressCompletedData represents the final result of a decompression operation.
type DecompressCompletedData struct {
	File       string `json:"file"`
	TotalFiles int64  `json:"total_files"`
	TotalBytes int64  `json:"total_bytes"`
	Success    bool   `json:"success"`
	Error      string `json:"error,omitempty"`
}

// NewDecompressProgress creates a new progress tracker for decompression.
func NewDecompressProgress(archiveFile string, callback func(DecompressProgressData)) *DecompressProgress {
	return &DecompressProgress{
		archiveFile:    archiveFile,
		callback:       callback,
		updateInterval: 250 * time.Millisecond,
		startTime:      time.Now(),
	}
}

// SetTotals sets the total file count and byte size for the archive.
func (p *DecompressProgress) SetTotals(totalFiles, totalBytes int64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.totalFiles = totalFiles
	p.totalBytes = totalBytes
}

// FileStarted is called when a new file extraction begins.
func (p *DecompressProgress) FileStarted(filename string) {
	p.mu.Lock()
	p.currentFile = filename
	p.mu.Unlock()
}

// FileCompleted is called when a file extraction is completed.
func (p *DecompressProgress) FileCompleted(bytesWritten int64) {
	p.processedFiles.Add(1)
	p.processedBytes.Add(bytesWritten)
	p.maybeEmitProgress()
}

// maybeEmitProgress emits a progress update if enough time has passed.
func (p *DecompressProgress) maybeEmitProgress() {
	if p.callback == nil {
		return
	}

	p.mu.Lock()
	now := time.Now()
	if now.Sub(p.lastUpdate) < p.updateInterval {
		p.mu.Unlock()
		return
	}
	p.lastUpdate = now
	data := p.getProgressDataLocked()
	p.mu.Unlock()

	p.callback(data)
}

// ForceEmit forces a progress update emission regardless of interval.
func (p *DecompressProgress) ForceEmit() {
	if p.callback == nil {
		return
	}
	p.mu.Lock()
	data := p.getProgressDataLocked()
	p.lastUpdate = time.Now()
	p.mu.Unlock()
	p.callback(data)
}

// getProgressDataLocked returns current progress data. Must be called with mu held.
func (p *DecompressProgress) getProgressDataLocked() DecompressProgressData {
	processedBytes := p.processedBytes.Load()
	processedFiles := p.processedFiles.Load()

	var progress float64
	if p.totalBytes > 0 {
		progress = float64(processedBytes) / float64(p.totalBytes)
	} else if p.totalFiles > 0 {
		progress = float64(processedFiles) / float64(p.totalFiles)
	}

	if progress > 1.0 {
		progress = 1.0
	}

	return DecompressProgressData{
		File:           p.archiveFile,
		CurrentFile:    p.currentFile,
		TotalFiles:     p.totalFiles,
		ProcessedFiles: processedFiles,
		TotalBytes:     p.totalBytes,
		ProcessedBytes: processedBytes,
		Progress:       progress,
		Percent:        int(progress * 100),
	}
}

// GetProgress returns the current progress data.
func (p *DecompressProgress) GetProgress() DecompressProgressData {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.getProgressDataLocked()
}

// CompletedData returns the completion data for the operation.
func (p *DecompressProgress) CompletedData(err error) DecompressCompletedData {
	p.mu.RLock()
	defer p.mu.RUnlock()

	data := DecompressCompletedData{
		File:       p.archiveFile,
		TotalFiles: p.processedFiles.Load(),
		TotalBytes: p.processedBytes.Load(),
		Success:    err == nil,
	}

	if err != nil {
		data.Error = err.Error()
	}

	return data
}
