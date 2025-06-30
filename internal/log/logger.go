// Package slog provee un logger de “nivel” sencillo con colores ANSI.
// Uso:
//
//	slog.Info("arrancó el servidor en %s", addr)
//	slog.SetLevel(slog.DebugLevel)
//
// Niveles por defecto: INFO, WARN, ERROR; DEBUG se activa con LOG_LEVEL=debug
package log

import (
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"
)

// Level representa severidad.
type Level int

const (
	DebugLevel Level = iota
	InfoLevel
	WarnLevel
	ErrorLevel
)

var levelNames = [...]string{"DEBUG", "INFO", "WARN", "ERROR"}

// Colores ANSI básicos.
var color = map[Level]string{
	DebugLevel: "\033[36m", // cyan
	InfoLevel:  "\033[32m", // green
	WarnLevel:  "\033[33m", // yellow
	ErrorLevel: "\033[31m", // red
}

const reset = "\033[0m"

var (
	minLevel   = InfoLevel
	logger     = log.New(os.Stdout, "", 0)
	levelMutex sync.RWMutex
)

// init lee LOG_LEVEL.
func init() {
	if env := strings.ToLower(os.Getenv("LOG_LEVEL")); env != "" {
		switch env {
		case "debug":
			minLevel = DebugLevel
		case "info":
			minLevel = InfoLevel
		case "warn", "warning":
			minLevel = WarnLevel
		case "error":
			minLevel = ErrorLevel
		}
	}
}

// SetLevel permite cambiarlo en caliente.
func SetLevel(l Level) { levelMutex.Lock(); minLevel = l; levelMutex.Unlock() }

// logf central.
func logf(lvl Level, format string, a ...interface{}) {
	levelMutex.RLock()
	defer levelMutex.RUnlock()
	if lvl < minLevel {
		return
	}
	ts := time.Now().Format("15:04:05.000")
	msg := fmt.Sprintf(format, a...)
	prefix := fmt.Sprintf("%s[%s] %s%s ", color[lvl], levelNames[lvl], ts, reset)
	logger.Printf(prefix + msg)
}

// Helpers públicos.
func Debug(format string, a ...interface{}) { logf(DebugLevel, format, a...) }
func Info(format string, a ...interface{})  { logf(InfoLevel, format, a...) }
func Warn(format string, a ...interface{})  { logf(WarnLevel, format, a...) }
func Error(format string, a ...interface{}) { logf(ErrorLevel, format, a...) }
