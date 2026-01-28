package filesystem

import (
	"sync"
	"sync/atomic"
	"time"
)

// CompressProgress tracks the progress of a compression operation.
// It provides real-time progress updates that can be consumed via WebSocket events.
//
// # WebSocket Events
//
// The compression progress is reported via two WebSocket events:
//
// ## "compress progress" event
//
// Sent periodically during compression with the following JSON payload:
//
//	{
//	    "total_files": 150,                 // Total number of files to compress
//	    "processed_files": 45,              // Number of files already compressed
//	    "total_bytes": 104857600,           // Total size of source files in bytes
//	    "processed_bytes": 31457280,        // Bytes already processed
//	    "progress": 0.30,                   // Progress as decimal (0.0 to 1.0)
//	    "percent": 30                       // Progress as percentage (0 to 100)
//	}
//
// ## "compress completed" event
//
// Sent when compression finishes (success or failure):
//
//	{
//	    "total_files": 150,
//	    "total_bytes": 104857600,
//	    "archive_size": 52428800,           // Size of the resulting archive
//	    "success": true,
//	    "error": ""                         // Error message if success is false
//	}
type CompressProgress struct {
	mu sync.RWMutex

	// Total number of files to compress
	totalFiles int64

	// Number of files already processed
	processedFiles atomic.Int64

	// Total size of source files in bytes
	totalBytes int64

	// Bytes already processed
	processedBytes atomic.Int64

	// Size of the resulting archive
	archiveSize int64

	// Start time of the operation
	startTime time.Time

	// Callback function for progress updates
	callback func(CompressProgressData)

	// Minimum interval between progress updates
	updateInterval time.Duration

	// Last time progress was reported
	lastUpdate time.Time
}

// CompressProgressData represents a snapshot of compression progress.
type CompressProgressData struct {
	TotalFiles     int64   `json:"total_files"`
	ProcessedFiles int64   `json:"processed_files"`
	TotalBytes     int64   `json:"total_bytes"`
	ProcessedBytes int64   `json:"processed_bytes"`
	Progress       float64 `json:"progress"`
	Percent        int     `json:"percent"`
}

// CompressCompletedData represents the final result of a compression operation.
type CompressCompletedData struct {
	TotalFiles  int64  `json:"total_files"`
	TotalBytes  int64  `json:"total_bytes"`
	ArchiveSize int64  `json:"archive_size"`
	Success     bool   `json:"success"`
	Error       string `json:"error,omitempty"`
}

// NewCompressProgress creates a new progress tracker for compression.
func NewCompressProgress(callback func(CompressProgressData)) *CompressProgress {
	return &CompressProgress{
		callback:       callback,
		updateInterval: 250 * time.Millisecond,
		startTime:      time.Now(),
	}
}

// SetTotals sets the total file count and byte size for the compression.
func (p *CompressProgress) SetTotals(totalFiles, totalBytes int64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.totalFiles = totalFiles
	p.totalBytes = totalBytes
}

// FileCompleted is called when a file compression is completed.
func (p *CompressProgress) FileCompleted(bytesProcessed int64) {
	p.processedFiles.Add(1)
	p.processedBytes.Add(bytesProcessed)
	p.maybeEmitProgress()
}

// AddBytes adds processed bytes without incrementing file count.
// Useful for tracking bytes within a single file.
func (p *CompressProgress) AddBytes(bytes int64) {
	p.processedBytes.Add(bytes)
	p.maybeEmitProgress()
}

// maybeEmitProgress emits a progress update if enough time has passed.
func (p *CompressProgress) maybeEmitProgress() {
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
func (p *CompressProgress) ForceEmit() {
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
func (p *CompressProgress) getProgressDataLocked() CompressProgressData {
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

	return CompressProgressData{
		TotalFiles:     p.totalFiles,
		ProcessedFiles: processedFiles,
		TotalBytes:     p.totalBytes,
		ProcessedBytes: processedBytes,
		Progress:       progress,
		Percent:        int(progress * 100),
	}
}

// GetProgress returns the current progress data.
func (p *CompressProgress) GetProgress() CompressProgressData {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.getProgressDataLocked()
}

// SetArchiveSize sets the final archive size.
func (p *CompressProgress) SetArchiveSize(size int64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.archiveSize = size
}

// CompletedData returns the completion data for the operation.
func (p *CompressProgress) CompletedData(err error) CompressCompletedData {
	p.mu.RLock()
	defer p.mu.RUnlock()

	data := CompressCompletedData{
		TotalFiles:  p.processedFiles.Load(),
		TotalBytes:  p.processedBytes.Load(),
		ArchiveSize: p.archiveSize,
		Success:     err == nil,
	}

	if err != nil {
		data.Error = err.Error()
	}

	return data
}
