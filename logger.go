// Package gokit provides shared authentication and billing utilities for microservices.
package gokit

// Logger is a minimal logging interface used throughout the kit library.
// Importing services can provide their own implementation.
type Logger interface {
	Debug(msg string, keys ...any)
	Info(msg string, keys ...any)
	Warn(msg string, keys ...any)
	Error(msg string, keys ...any)
}

// NoOpLogger is a no-op implementation of Logger.
type NoOpLogger struct{}

func (n NoOpLogger) Debug(msg string, keys ...any) {}
func (n NoOpLogger) Info(msg string, keys ...any)  {}
func (n NoOpLogger) Warn(msg string, keys ...any)  {}
func (n NoOpLogger) Error(msg string, keys ...any) {}

var globalLogger Logger = NoOpLogger{}

// SetLogger sets the global logger for the kit library.
func SetLogger(l Logger) {
	globalLogger = l
}

// GetLogger returns the global logger.
func GetLogger() Logger {
	return globalLogger
}
