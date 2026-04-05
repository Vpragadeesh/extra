package downloader

import (
	"io"
	"os"
)

// Worker handles chunk downloads
type Worker struct {
	id       int
	file     *os.File
	offset   int64
	size     int64
	done     chan error
}

// NewWorker creates a new worker
func NewWorker(id int, file *os.File, offset, size int64) *Worker {
	return &Worker{
		id:     id,
		file:   file,
		offset: offset,
		size:   size,
		done:   make(chan error, 1),
	}
}

// Download downloads a chunk
func (w *Worker) Download(reader io.Reader) error {
	// Seek to offset
	if _, err := w.file.Seek(w.offset, 0); err != nil {
		return err
	}

	// Write bytes
	_, err := io.CopyN(w.file, reader, w.size)
	if err != nil && err != io.EOF {
		return err
	}
	return nil
}
