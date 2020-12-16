package logging

import (
	"log"
	"os"
	"path/filepath"
	"runtime"

	"github.com/redforks/xdgdirs"
)

// GetLogDir returns os specific directory to store log files.
func GetLogDir() string {
	switch runtime.GOOS {
	case "linux", "darwin":
		if os.Getuid() == 0 {
			return "/var/log"
		}
		return filepath.Join(xdgdirs.Home(), ".local", "log")
	default:
		log.Panicf("[%s] GetDataDir do not support OS: %s", tag, runtime.GOOS)
		return ""
	}
}
