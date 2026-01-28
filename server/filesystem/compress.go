package filesystem

import (
	"context"
	"fmt"
	"io"
	iofs "io/fs"
	"path"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"emperror.dev/errors"
	"github.com/klauspost/compress/zip"
	"github.com/mholt/archives"

	"github.com/pterodactyl/wings/internal/ufs"
	"github.com/pterodactyl/wings/server/filesystem/archiverext"
)

// CompressFiles compresses all the files matching the given paths in the
// specified directory. This function also supports passing nested paths to only
// compress certain files and folders when working in a larger directory. This
// effectively creates a local backup, but rather than ignoring specific files
// and folders, it takes an allow-list of files and folders.
//
// All paths are relative to the dir that is passed in as the first argument,
// and the compressed file will be placed at that location named
// `archive-{date}.tar.gz`.
func (fs *Filesystem) CompressFiles(dir string, paths []string) (ufs.FileInfo, error) {
	return fs.CompressFilesWithProgress(context.Background(), dir, paths, nil)
}

// CompressFilesWithProgress compresses files while reporting progress.
// The progress tracker is called periodically with progress updates.
// If progress is nil, no progress updates are reported.
func (fs *Filesystem) CompressFilesWithProgress(ctx context.Context, dir string, paths []string, progress *CompressProgress) (ufs.FileInfo, error) {
	a := &Archive{Filesystem: fs, BaseDirectory: dir, Files: paths, CompProgress: progress}
	d := path.Join(
		dir,
		fmt.Sprintf("archive-%s.tar.gz", strings.ReplaceAll(time.Now().Format(time.RFC3339), ":", "")),
	)
	f, err := fs.unixFS.OpenFile(d, ufs.O_WRONLY|ufs.O_CREATE, 0o644)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	cw := ufs.NewCountedWriter(f)
	if err := a.Stream(ctx, cw); err != nil {
		return nil, err
	}
	if !fs.unixFS.CanFit(cw.BytesWritten()) {
		_ = fs.unixFS.Remove(d)
		return nil, newFilesystemError(ErrCodeDiskSpace, nil)
	}
	fs.unixFS.Add(cw.BytesWritten())

	// Set the archive size for progress tracking
	if progress != nil {
		progress.SetArchiveSize(cw.BytesWritten())
	}

	return f.Stat()
}

func (fs *Filesystem) archiverFileSystem(ctx context.Context, p string) (iofs.FS, error) {
	f, err := fs.unixFS.Open(p)
	if err != nil {
		return nil, err
	}
	// Do not use defer to close `f`, it will likely be used later.

	format, _, err := archives.Identify(ctx, filepath.Base(p), f)
	if err != nil && !errors.Is(err, archives.NoMatch) {
		_ = f.Close()
		return nil, err
	}

	// Reset the file reader.
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		_ = f.Close()
		return nil, err
	}

	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, err
	}

	if format != nil {
		switch ff := format.(type) {
		case archives.Zip:
			// zip.Reader is more performant than ArchiveFS, because zip.Reader caches content information
			// and zip.Reader can open several content files concurrently because of io.ReaderAt requirement
			// while ArchiveFS can't.
			// zip.Reader doesn't suffer from issue #330 and #310 according to local test (but they should be fixed anyway)
			return zip.NewReader(f, info.Size())
		case archives.Extraction:
			return &archives.ArchiveFS{Stream: io.NewSectionReader(f, 0, info.Size()), Format: ff, Context: ctx}, nil
		case archives.Compression:
			return archiverext.FileFS{File: f, Compression: ff}, nil
		}
	}
	_ = f.Close()
	return nil, archives.NoMatch
}

// ArchiveInfo contains metadata about an archive file or files to be archived.
type ArchiveInfo struct {
	TotalFiles int64
	TotalBytes int64
}

// GetFilesInfo calculates the total file count and size for a list of files/directories.
func (fs *Filesystem) GetFilesInfo(dir string, paths []string) (ArchiveInfo, error) {
	var info ArchiveInfo
	for _, p := range paths {
		fullPath := path.Join(dir, p)
		stat, err := fs.Stat(fullPath)
		if err != nil {
			continue
		}

		if stat.IsDir() {
			dirInfo, err := fs.getDirectoryInfo(fullPath)
			if err != nil {
				continue
			}
			info.TotalFiles += dirInfo.TotalFiles
			info.TotalBytes += dirInfo.TotalBytes
		} else {
			info.TotalFiles++
			info.TotalBytes += stat.Size()
		}
	}
	return info, nil
}

// getDirectoryInfo recursively calculates file count and size for a directory.
func (fs *Filesystem) getDirectoryInfo(dir string) (ArchiveInfo, error) {
	var info ArchiveInfo
	entries, err := fs.ListDirectory(dir)
	if err != nil {
		return info, err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			subInfo, err := fs.getDirectoryInfo(path.Join(dir, entry.Name()))
			if err != nil {
				continue
			}
			info.TotalFiles += subInfo.TotalFiles
			info.TotalBytes += subInfo.TotalBytes
		} else {
			info.TotalFiles++
			info.TotalBytes += entry.Size()
		}
	}
	return info, nil
}

// GetArchiveInfo analyzes an archive and returns information about its contents.
func (fs *Filesystem) GetArchiveInfo(ctx context.Context, dir string, file string) (ArchiveInfo, error) {
	fsys, err := fs.archiverFileSystem(ctx, filepath.Join(dir, file))
	if err != nil {
		if errors.Is(err, archives.NoMatch) {
			return ArchiveInfo{}, newFilesystemError(ErrCodeUnknownArchive, err)
		}
		return ArchiveInfo{}, err
	}

	var info ArchiveInfo
	err = iofs.WalkDir(fsys, ".", func(path string, d iofs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			if !d.IsDir() {
				info.TotalFiles++
				if fi, err := d.Info(); err == nil {
					info.TotalBytes += fi.Size()
				}
			}
			return nil
		}
	})
	return info, err
}

// SpaceAvailableForDecompression looks through a given archive and determines
// if decompressing it would put the server over its allocated disk space limit.
func (fs *Filesystem) SpaceAvailableForDecompression(ctx context.Context, dir string, file string) error {
	// Don't waste time trying to determine this if we know the server will have the space for
	// it since there is no limit.
	if fs.MaxDisk() <= 0 {
		return nil
	}

	fsys, err := fs.archiverFileSystem(ctx, filepath.Join(dir, file))
	if err != nil {
		if errors.Is(err, archives.NoMatch) {
			return newFilesystemError(ErrCodeUnknownArchive, err)
		}
		return err
	}

	var size atomic.Int64
	return iofs.WalkDir(fsys, ".", func(path string, d iofs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		select {
		case <-ctx.Done():
			// Stop walking if the context is canceled.
			return ctx.Err()
		default:
			info, err := d.Info()
			if err != nil {
				return err
			}
			if !fs.unixFS.CanFit(size.Add(info.Size())) {
				return newFilesystemError(ErrCodeDiskSpace, nil)
			}
			return nil
		}
	})
}

// DecompressFile will decompress a file in a given directory by using the
// archiver tool to infer the file type and go from there. This will walk over
// all the files within the given archive and ensure that there is not a
// zip-slip attack being attempted by validating that the final path is within
// the server data directory.
func (fs *Filesystem) DecompressFile(ctx context.Context, dir string, file string) error {
	return fs.DecompressFileWithProgress(ctx, dir, file, nil)
}

// DecompressFileWithProgress decompresses an archive while reporting progress.
// The progress callback is called periodically with progress updates.
// If progress is nil, no progress updates are reported.
func (fs *Filesystem) DecompressFileWithProgress(ctx context.Context, dir string, file string, progress *DecompressProgress) error {
	f, err := fs.unixFS.Open(filepath.Join(dir, file))
	if err != nil {
		return err
	}
	defer f.Close()

	// Identify the type of archive we are dealing with.
	format, input, err := archives.Identify(ctx, filepath.Base(file), f)
	if err != nil {
		if errors.Is(err, archives.NoMatch) {
			return newFilesystemError(ErrCodeUnknownArchive, err)
		}
		return err
	}

	return fs.extractStream(ctx, extractStreamOptions{
		FileName:  file,
		Directory: dir,
		Format:    format,
		Reader:    input,
		Progress:  progress,
	})
}

// ExtractStreamUnsafe .
func (fs *Filesystem) ExtractStreamUnsafe(ctx context.Context, dir string, r io.Reader) error {
	format, input, err := archives.Identify(ctx, "archive.tar.gz", r)
	if err != nil {
		if errors.Is(err, archives.NoMatch) {
			return newFilesystemError(ErrCodeUnknownArchive, err)
		}
		return err
	}
	return fs.extractStream(ctx, extractStreamOptions{
		Directory: dir,
		Format:    format,
		Reader:    input,
	})
}

type extractStreamOptions struct {
	// The directory to extract the archive to.
	Directory string
	// File name of the archive.
	FileName string
	// Format of the archive.
	Format archives.Format
	// Reader for the archive.
	Reader io.Reader
	// Progress tracker for reporting extraction progress.
	Progress *DecompressProgress
}

func (fs *Filesystem) extractStream(ctx context.Context, opts extractStreamOptions) error {
	// See if it's a compressed archive, such as TAR or a ZIP
	ex, ok := opts.Format.(archives.Extractor)
	if !ok {
		// If not, check if it's a single-file compression, such as
		// .log.gz, .sql.gz, and so on
		de, ok := opts.Format.(archives.Decompressor)
		if !ok {
			return nil
		}

		// Strip the compression suffix
		p := filepath.Join(opts.Directory, strings.TrimSuffix(opts.FileName, opts.Format.Extension()))

		// Make sure it's not ignored
		if err := fs.IsIgnored(p); err != nil {
			return nil
		}

		reader, err := de.OpenReader(opts.Reader)
		if err != nil {
			return err
		}
		defer reader.Close()

		// Open the file for creation/writing
		f, err := fs.unixFS.OpenFile(p, ufs.O_WRONLY|ufs.O_CREATE, 0o644)
		if err != nil {
			return err
		}
		defer f.Close()

		// For single-file compression, set progress for 1 file
		if opts.Progress != nil {
			opts.Progress.FileStarted(p)
		}

		var totalWritten int64
		// Read in 4 KB chunks
		buf := make([]byte, 4096)
		for {
			n, err := reader.Read(buf)
			if n > 0 {

				// Check quota before writing the chunk
				if quotaErr := fs.HasSpaceFor(int64(n)); quotaErr != nil {
					return quotaErr
				}

				// Write the chunk
				if _, writeErr := f.Write(buf[:n]); writeErr != nil {
					return writeErr
				}

				// Add to quota
				fs.addDisk(int64(n))
				totalWritten += int64(n)
			}

			if err != nil {
				// EOF are expected
				if err == io.EOF {
					break
				}

				// Return any other
				return err
			}
		}

		// Report completion for single-file decompression
		if opts.Progress != nil {
			opts.Progress.FileCompleted(totalWritten)
			opts.Progress.ForceEmit()
		}

		return nil
	}

	// Decompress and extract archive
	return ex.Extract(ctx, opts.Reader, func(ctx context.Context, f archives.FileInfo) error {
		if f.IsDir() {
			return nil
		}
		p := filepath.Join(opts.Directory, f.NameInArchive)
		// If it is ignored, just don't do anything with the file and skip over it.
		if err := fs.IsIgnored(p); err != nil {
			return nil
		}

		// Report file start to progress tracker
		if opts.Progress != nil {
			opts.Progress.FileStarted(f.NameInArchive)
		}

		r, err := f.Open()
		if err != nil {
			return err
		}
		defer r.Close()
		if err := fs.Write(p, r, f.Size(), f.Mode()); err != nil {
			return wrapError(err, opts.FileName)
		}
		// Update the file modification time to the one set in the archive.
		if err := fs.Chtimes(p, f.ModTime(), f.ModTime()); err != nil {
			return wrapError(err, opts.FileName)
		}

		// Report file completion to progress tracker
		if opts.Progress != nil {
			opts.Progress.FileCompleted(f.Size())
		}

		return nil
	})
}
