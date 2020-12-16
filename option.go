// Package logging package defines AsyncLogWriter, and FileLogWriter can be used as
// log.SetOutput() argument.
//
// Support auto config through spork/config. By default, only enable stdout as
// log output, by provide log filename, will enable file log, and automatic log
// file management (see FileLogWriter for details).
package logging

import (
	"io"
	"log"
	"os"
	"path/filepath"

	"github.com/redforks/appinfo"
	"github.com/redforks/config"
)

const (
	tag = "logging"
)

// KNOWN Bug:
//
//  Must Set ToConsole to false, if run with -d, --daemon flag.
//  In daemon mode, stdout is closed, then write to console will fail,
//  and MultiWriter will fail too, hence FileWriter won't called anymore.
type option struct {
	ToConsole bool // if true, also log to stderr
	ToFile    bool // if true, enable Async log file

	LogFile          string // if "", use /var/log/[AppName].log
	MaxLogFileLen    int64  // max log file size, if reached, rename and create new file. Old file compressed
	MaxArchivedFiles int    // How many compressed file kept.
}

func (o *option) Init() error {
	var writers []io.Writer
	if o.ToConsole {
		writers = append(writers, os.Stdout)
	}
	if o.ToFile {
		fn := o.LogFile
		if fn == "" {
			fn = filepath.Join(GetLogDir(), appinfo.CodeName()+".log")
		}
		w, err := NewFileLogWriter(fn, o.MaxLogFileLen, o.MaxArchivedFiles)
		if err != nil {
			return err
		}

		log.Printf("[%s] write log to %s", tag, o.LogFile)
		w = NewAsyncLogWriter(w)
		writers = append(writers, w)
	}

	var w io.Writer
	switch len(writers) {
	case 0:
	case 1:
		w = writers[0]
	default:
		w = io.MultiWriter(writers...)
	}

	if w != nil {
		log.SetOutput(w)
	}
	return nil
}

func (o *option) Apply() {
	log.Printf("[%s] not support apply changed options, must restart to take effect.", tag)
}

func init() {
	config.Register("logging", func() config.Option {
		return &option{
			ToConsole:        true,
			ToFile:           true,
			MaxLogFileLen:    256 * 1024 * 1024,
			MaxArchivedFiles: 5,
		}
	})
}
