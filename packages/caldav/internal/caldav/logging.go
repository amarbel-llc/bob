package caldav

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

func logDir() string {
	base := os.Getenv("XDG_LOG_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			home = "."
		}
		base = filepath.Join(home, ".local", "log")
	}
	return filepath.Join(base, "caldav")
}

// InitLogging creates the log directory and opens a log file at
// $XDG_LOG_HOME/caldav/caldav.log. Returns a logger that writes to both
// stderr and the log file, plus a close function. If the log file cannot
// be opened, the logger writes to stderr only.
func InitLogging() (*log.Logger, func()) {
	dir := logDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not create log directory %s: %v\n", dir, err)
		return log.New(os.Stderr, "", log.LstdFlags), func() {}
	}

	logPath := filepath.Join(dir, "caldav.log")
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not open log file %s: %v\n", logPath, err)
		return log.New(os.Stderr, "", log.LstdFlags), func() {}
	}

	w := io.MultiWriter(os.Stderr, f)
	return log.New(w, "", log.LstdFlags), func() { f.Close() }
}

type loggingTransport struct {
	base http.RoundTripper
	log  *log.Logger
}

func (t *loggingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	start := time.Now()
	t.log.Printf("caldav: %s %s", req.Method, req.URL.Path)

	resp, err := t.base.RoundTrip(req)
	elapsed := time.Since(start)

	if err != nil {
		t.log.Printf("caldav: %s %s -> error: %v (%s)", req.Method, req.URL.Path, err, elapsed)
		return nil, err
	}

	t.log.Printf("caldav: %s %s -> %d (%s)", req.Method, req.URL.Path, resp.StatusCode, elapsed)
	return resp, nil
}
