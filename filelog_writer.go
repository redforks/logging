package logging

import (
	"compress/gzip"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"

	"github.com/redforks/hal"
)

// Log writer manage log files:
//
//  1. Create new file if log file length is too large
//  2. Compress old log files (Done in its own goroutine).
//
// fileLogWriter should wrapped in AsyncLogWriter, to prevent hurt log caller's
// performance.
type fileLogWriter struct {
	path     string // log file path
	f        *os.File
	maxLen   int64
	maxFiles int
}

// NewFileLogWriter create a new instance fileLogWriter.
// maxLen: If log file length greater than maxLen, a new log file created
// maxFiles: Limits of archived files, old archived files will delete.
func NewFileLogWriter(path string, maxLen int64, maxFiles int) (io.Writer, error) {
	f, err := openLogFile(path)
	if err != nil {
		return nil, err
	}
	r := &fileLogWriter{path, f, maxLen, maxFiles}
	r.recoverPartialCompressFiles(path)
	return r, nil
}

func (w *fileLogWriter) recoverPartialCompressFiles(path string) {
	unCompressed, err := w.getUncompressedFiles(path)
	if err != nil {
		logError(err)
		return
	}

	for _, item := range unCompressed {
		go func(f string) {
			if err := w.compress(f); err != nil {
				logError(err)
			}
		}(item)
	}
}

func (w *fileLogWriter) Write(p []byte) (n int, err error) {
	if n, err = w.f.Write(p); err != nil {
		return
	}

	size, err := w.fileSize()
	if err != nil {
		return
	}

	if size >= w.maxLen {
		fname := w.f.Name()
		if err = w.f.Close(); err != nil {
			return
		}
		bakFile := w.newBackupFilename(fname)
		if err = os.Rename(fname, w.newBackupFilename(fname)); err != nil {
			return
		}
		if w.f, err = openLogFile(fname); err != nil {
			return
		}

		go func() {
			if err := w.compress(bakFile); err != nil {
				logError(err)
			} else {
				if err := w.cleanOldBackupFiles(fname); err != nil {
					logError(err)
				}
			}
		}()
	}
	return
}

func (w *fileLogWriter) compress(logFile string) error {
	gzfile := logFile + `.gz`
	f, err := os.Create(gzfile)
	if err != nil {
		return err
	}
	defer safeClose(f)

	src, err := os.Open(logFile)
	if err != nil {
		return err
	}
	defer safeClose(src)

	dest := gzip.NewWriter(f)
	_, err = io.Copy(dest, src)
	if err != nil {
		return err
	}
	if err := dest.Close(); err != nil {
		return err
	}
	return os.Remove(logFile)
}

func safeClose(f io.Closer) {
	if err := f.Close(); err != nil {
		logError(err)
	}
}

func logError(err error) {
	s := err.Error()
	_, _ = os.Stderr.Write([]byte(s))
}

func (w *fileLogWriter) cleanOldBackupFiles(logfilename string) error {
	files, err := w.getCompressedFiles(logfilename)
	if err != nil {
		return err
	}

	for i := 0; i < len(files)-w.maxFiles; i++ {
		if err = os.Remove(files[i]); err != nil {
			return err
		}
	}
	return nil
}

func (w *fileLogWriter) getCompressedFiles(logfilename string) ([]string, error) {
	return w.getBackFiles(logfilename, `.gz`)
}

func (w *fileLogWriter) getUncompressedFiles(logfilename string) ([]string, error) {
	return w.getBackFiles(logfilename, ``)
}

func (w *fileLogWriter) getBackFiles(logfilename, suffix string) ([]string, error) {
	base, ext := w.splitLogFilename(logfilename)
	files, err := filepath.Glob(base + `-*` + ext + suffix)
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	return files, nil
}

// Split the log filename to two parts: withoutExt, ext.
func (w *fileLogWriter) splitLogFilename(logfilename string) (without, ext string) {
	ext = filepath.Ext(logfilename)
	return logfilename[:len(logfilename)-len(ext)], ext
}

func (w *fileLogWriter) newBackupFilename(logfilename string) string {
	ext := filepath.Ext(logfilename)
	return fmt.Sprintf(`%s-%s%s`, logfilename[:len(logfilename)-len(ext)], hal.Now().Format(`2006-01-02-150405`), ext)
}

func (w *fileLogWriter) fileSize() (int64, error) {
	info, err := w.f.Stat()
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

func openLogFile(path string) (f *os.File, err error) {
	f, err = os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, os.ModeAppend|os.ModePerm)
	if err != nil && os.IsNotExist(err) {
		if e := os.MkdirAll(filepath.Dir(path), 0700); e != nil {
			log.Printf("[%s] Create log directory failed: %s", tag, e)
			return
		}
		f, err = os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, os.ModeAppend|os.ModePerm)
	}
	return
}
