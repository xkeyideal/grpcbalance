// Copyright 2016 The etcd Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package logger provides a simple logging interface for grpclient.
package logger

import (
	"log"
	"os"
)

// Logger is the interface for logging in grpclient.
// It provides a minimal set of methods that any logging implementation should support.
type Logger interface {
	// Debug logs a message at debug level.
	Debug(args ...interface{})
	// Debugf logs a formatted message at debug level.
	Debugf(format string, args ...interface{})

	// Info logs a message at info level.
	Info(args ...interface{})
	// Infof logs a formatted message at info level.
	Infof(format string, args ...interface{})

	// Warn logs a message at warn level.
	Warn(args ...interface{})
	// Warnf logs a formatted message at warn level.
	Warnf(format string, args ...interface{})

	// Error logs a message at error level.
	Error(args ...interface{})
	// Errorf logs a formatted message at error level.
	Errorf(format string, args ...interface{})
}

// Level represents the logging level.
type Level int

const (
	LevelDebug Level = iota
	LevelInfo
	LevelWarn
	LevelError
	LevelSilent // Disables all logging
)

// defaultLogger is the default implementation of Logger using Go's standard log package.
type defaultLogger struct {
	level  Level
	logger *log.Logger
}

// NewDefaultLogger creates a new default logger with the specified level.
func NewDefaultLogger(level Level) Logger {
	return &defaultLogger{
		level:  level,
		logger: log.New(os.Stderr, "[grpclient] ", log.LstdFlags|log.Lmsgprefix),
	}
}

// NewDefaultLoggerWithPrefix creates a new default logger with a custom prefix.
func NewDefaultLoggerWithPrefix(level Level, prefix string) Logger {
	return &defaultLogger{
		level:  level,
		logger: log.New(os.Stderr, prefix, log.LstdFlags|log.Lmsgprefix),
	}
}

func (l *defaultLogger) Debug(args ...interface{}) {
	if l.level <= LevelDebug {
		l.logger.Println(append([]interface{}{"[DEBUG]"}, args...)...)
	}
}

func (l *defaultLogger) Debugf(format string, args ...interface{}) {
	if l.level <= LevelDebug {
		l.logger.Printf("[DEBUG] "+format, args...)
	}
}

func (l *defaultLogger) Info(args ...interface{}) {
	if l.level <= LevelInfo {
		l.logger.Println(append([]interface{}{"[INFO]"}, args...)...)
	}
}

func (l *defaultLogger) Infof(format string, args ...interface{}) {
	if l.level <= LevelInfo {
		l.logger.Printf("[INFO] "+format, args...)
	}
}

func (l *defaultLogger) Warn(args ...interface{}) {
	if l.level <= LevelWarn {
		l.logger.Println(append([]interface{}{"[WARN]"}, args...)...)
	}
}

func (l *defaultLogger) Warnf(format string, args ...interface{}) {
	if l.level <= LevelWarn {
		l.logger.Printf("[WARN] "+format, args...)
	}
}

func (l *defaultLogger) Error(args ...interface{}) {
	if l.level <= LevelError {
		l.logger.Println(append([]interface{}{"[ERROR]"}, args...)...)
	}
}

func (l *defaultLogger) Errorf(format string, args ...interface{}) {
	if l.level <= LevelError {
		l.logger.Printf("[ERROR] "+format, args...)
	}
}

// NopLogger is a no-op logger that discards all log messages.
type NopLogger struct{}

func (NopLogger) Debug(args ...interface{})                 {}
func (NopLogger) Debugf(format string, args ...interface{}) {}
func (NopLogger) Info(args ...interface{})                  {}
func (NopLogger) Infof(format string, args ...interface{})  {}
func (NopLogger) Warn(args ...interface{})                  {}
func (NopLogger) Warnf(format string, args ...interface{})  {}
func (NopLogger) Error(args ...interface{})                 {}
func (NopLogger) Errorf(format string, args ...interface{}) {}

// NewNopLogger returns a logger that discards all log messages.
func NewNopLogger() Logger {
	return NopLogger{}
}

// defaultGlobalLogger is the package-level default logger.
var defaultGlobalLogger Logger = NewDefaultLogger(LevelInfo)

// SetDefaultLogger sets the package-level default logger.
func SetDefaultLogger(l Logger) {
	if l != nil {
		defaultGlobalLogger = l
	}
}

func SetDefaultLoggerLevel(level Level) {
}

// GetDefaultLogger returns the package-level default logger.
func GetDefaultLogger() Logger {
	return defaultGlobalLogger
}
