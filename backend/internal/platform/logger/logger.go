package logger

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sync"
	"time"
)

// ANSI color codes
const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorBlue   = "\033[34m"
	colorPurple = "\033[35m"
	colorCyan   = "\033[36m"
	colorGray   = "\033[90m"
	colorWhite  = "\033[97m"

	colorBoldRed    = "\033[1;31m"
	colorBoldGreen  = "\033[1;32m"
	colorBoldYellow = "\033[1;33m"
	colorBoldBlue   = "\033[1;34m"
	colorBoldCyan   = "\033[1;36m"
)

// Logger is an interface for structured logging.
type Logger interface {
	Debug(msg string, args ...any)
	Info(msg string, args ...any)
	Warn(msg string, args ...any)
	Error(msg string, args ...any)
	Fatal(msg string, args ...any)
	With(args ...any) Logger
	WithGroup(name string) Logger
	DebugContext(ctx context.Context, msg string, args ...any)
	InfoContext(ctx context.Context, msg string, args ...any)
	WarnContext(ctx context.Context, msg string, args ...any)
	ErrorContext(ctx context.Context, msg string, args ...any)
}

// SlogLogger wraps slog.Logger and implements Logger.
type SlogLogger struct {
	slog *slog.Logger
}

// New creates a Logger. format: "json" | "plain" | "text" (colored, default).
// level: "debug" | "info" | "warn" | "error".
func New(format, level string) Logger {
	return NewWithWriter(format, level, os.Stdout)
}

// NewWithWriter is like New but writes to w. Useful for tests that need to
// inspect emitted log lines.
func NewWithWriter(format, level string, w io.Writer) Logger {
	opts := &slog.HandlerOptions{Level: parseLevel(level)}
	var handler slog.Handler
	switch format {
	case "json":
		handler = slog.NewJSONHandler(w, opts)
	case "plain":
		handler = slog.NewTextHandler(w, opts)
	default:
		handler = newColoredTextHandler(w, opts)
	}
	// Enrich *Context log calls with trace_id/span_id when a span is active.
	handler = newTraceContextHandler(handler)
	return &SlogLogger{slog: slog.New(handler)}
}

func (l *SlogLogger) With(args ...any) Logger       { return &SlogLogger{slog: l.slog.With(args...)} }
func (l *SlogLogger) WithGroup(name string) Logger  { return &SlogLogger{slog: l.slog.WithGroup(name)} }
func (l *SlogLogger) Debug(msg string, args ...any) { l.slog.Debug(msg, args...) }
func (l *SlogLogger) Info(msg string, args ...any)  { l.slog.Info(msg, args...) }
func (l *SlogLogger) Warn(msg string, args ...any)  { l.slog.Warn(msg, args...) }
func (l *SlogLogger) Error(msg string, args ...any) { l.slog.Error(msg, args...) }
func (l *SlogLogger) Fatal(msg string, args ...any) { l.slog.Error(msg, args...); os.Exit(1) }
func (l *SlogLogger) DebugContext(ctx context.Context, msg string, args ...any) {
	l.slog.DebugContext(ctx, msg, args...)
}

func (l *SlogLogger) InfoContext(ctx context.Context, msg string, args ...any) {
	l.slog.InfoContext(ctx, msg, args...)
}

func (l *SlogLogger) WarnContext(ctx context.Context, msg string, args ...any) {
	l.slog.WarnContext(ctx, msg, args...)
}

func (l *SlogLogger) ErrorContext(ctx context.Context, msg string, args ...any) {
	l.slog.ErrorContext(ctx, msg, args...)
}

// Err returns a slog.Attr for an error value.
func Err(err error) slog.Attr { return slog.Any("error", err) }

// defaultLogger is the package-level logger used by top-level functions.
var defaultLogger Logger = New("text", "info")

// SetDefault sets the global slog default using this Logger's underlying handler,
// and also sets the package-level default logger.
func SetDefault(l Logger) {
	defaultLogger = l
	if sl, ok := l.(*SlogLogger); ok {
		slog.SetDefault(sl.slog)
	}
}

// Package-level logging functions that delegate to the default logger.
func Debug(msg string, args ...any) { defaultLogger.Debug(msg, args...) }
func Info(msg string, args ...any)  { defaultLogger.Info(msg, args...) }
func Warn(msg string, args ...any)  { defaultLogger.Warn(msg, args...) }
func Error(msg string, args ...any) { defaultLogger.Error(msg, args...) }

func parseLevel(level string) slog.Level {
	switch level {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// coloredTextHandler outputs color-coded log lines to a writer.
type coloredTextHandler struct {
	opts   slog.HandlerOptions
	mu     *sync.Mutex
	out    io.Writer
	attrs  []slog.Attr
	groups []string
}

func newColoredTextHandler(out io.Writer, opts *slog.HandlerOptions) *coloredTextHandler {
	if opts == nil {
		opts = &slog.HandlerOptions{}
	}
	return &coloredTextHandler{opts: *opts, mu: &sync.Mutex{}, out: out}
}

func (h *coloredTextHandler) Enabled(_ context.Context, level slog.Level) bool {
	min := slog.LevelInfo
	if h.opts.Level != nil {
		min = h.opts.Level.Level()
	}
	return level >= min
}

func (h *coloredTextHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	levelColor, levelText := h.levelStyle(r.Level)
	var attrs string
	for _, a := range h.attrs {
		attrs += h.formatAttr(a)
	}
	r.Attrs(func(a slog.Attr) bool { attrs += h.formatAttr(a); return true })

	line := fmt.Sprintf("%s%s%s %s%-5s%s %s%s%s%s\n",
		colorGray, r.Time.Format(time.DateTime), colorReset,
		levelColor, levelText, colorReset,
		colorWhite, r.Message, colorReset,
		attrs,
	)
	_, err := h.out.Write([]byte(line))
	return err
}

func (h *coloredTextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	merged := make([]slog.Attr, len(h.attrs)+len(attrs))
	copy(merged, h.attrs)
	copy(merged[len(h.attrs):], attrs)
	return &coloredTextHandler{opts: h.opts, mu: h.mu, out: h.out, attrs: merged, groups: h.groups}
}

func (h *coloredTextHandler) WithGroup(name string) slog.Handler {
	groups := make([]string, len(h.groups)+1)
	copy(groups, h.groups)
	groups[len(h.groups)] = name
	return &coloredTextHandler{opts: h.opts, mu: h.mu, out: h.out, attrs: h.attrs, groups: groups}
}

func (h *coloredTextHandler) levelStyle(level slog.Level) (string, string) {
	switch {
	case level < slog.LevelInfo:
		return colorBoldCyan, "DEBUG"
	case level < slog.LevelWarn:
		return colorBoldGreen, "INFO"
	case level < slog.LevelError:
		return colorBoldYellow, "WARN"
	default:
		return colorBoldRed, "ERROR"
	}
}

func (h *coloredTextHandler) formatAttr(a slog.Attr) string {
	if a.Equal(slog.Attr{}) {
		return ""
	}
	key := a.Key
	for _, g := range h.groups {
		key = g + "." + key
	}
	v := a.Value.Resolve()
	var vc, vs string
	switch v.Kind() {
	case slog.KindString:
		vc, vs = colorGreen, v.String()
	case slog.KindInt64:
		vc, vs = colorPurple, fmt.Sprintf("%d", v.Int64())
	case slog.KindUint64:
		vc, vs = colorPurple, fmt.Sprintf("%d", v.Uint64())
	case slog.KindFloat64:
		vc, vs = colorPurple, fmt.Sprintf("%g", v.Float64())
	case slog.KindBool:
		vc, vs = colorYellow, fmt.Sprintf("%t", v.Bool())
	case slog.KindTime:
		vc, vs = colorGray, v.Time().Format(time.RFC3339)
	case slog.KindDuration:
		vc, vs = colorPurple, v.Duration().String()
	default:
		vc, vs = colorWhite, fmt.Sprintf("%v", v.Any())
	}
	return fmt.Sprintf(" %s%s%s=%s%s%s", colorCyan, key, colorReset, vc, vs, colorReset)
}
