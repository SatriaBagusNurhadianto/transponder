// Package telemetry provides the logger shared by the process.
package telemetry

import (
	"fmt"
	"io"
	"log/slog"
	"strings"
)

// NewLogger returns a JSON logger that writes to w.
// level is debug, info, warn or error, compared case-insensitively.
// An empty level means info. Any other value is an error that names it.
// Every record includes a service attribute set to transponder.
func NewLogger(w io.Writer, level string) (*slog.Logger, error) {
	lvl, err := parseLevel(level)
	if err != nil {
		return nil, err
	}
	handler := slog.NewJSONHandler(w, &slog.HandlerOptions{Level: lvl}).WithAttrs([]slog.Attr{
		slog.String("service", "transponder"),
	})
	return slog.New(handler), nil
}

func parseLevel(level string) (slog.Level, error) {
	switch strings.ToLower(level) {
	case "", "info":
		return slog.LevelInfo, nil
	case "debug":
		return slog.LevelDebug, nil
	case "warn":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("log level %q must be debug, info, warn or error", level)
	}
}
