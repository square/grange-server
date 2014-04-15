// Yes there are a million libraries that do logging, but for 50 lines of code
// I get things just the way I like them.

package main

import (
	"fmt"
	"time"
)

const (
	FATAL = iota
	WARN
	INFO
	DEBUG
)

var (
	logLevel = INFO
)

func Fatal(msg string, args ...interface{}) { logAt(FATAL, "FATAL", msg, args...) }
func Warn(msg string, args ...interface{})  { logAt(WARN, "WARN", msg, args...) }
func Info(msg string, args ...interface{})  { logAt(INFO, "INFO", msg, args...) }
func Debug(msg string, args ...interface{}) { logAt(DEBUG, "DEBUG", msg, args...) }

func logAt(level int, tag string, msg string, args ...interface{}) {
	if logLevel < level {
		return
	}

	Log(tag, msg, args...)
}

func Log(tag string, msg string, args ...interface{}) {
	now := time.Now().UTC()

	fmt.Printf("%-5s [%s] %s\n", tag,
		now.Format("2006-01-02 15:04:05.999"),
		fmt.Sprintf(msg, args...))
}
