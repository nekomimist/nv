package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"strconv"
)

type logLevel string

const (
	logLevelDebug logLevel = "debug"
	logLevelInfo  logLevel = "info"
	logLevelWarn  logLevel = "warn"
	logLevelError logLevel = "error"
	logLevelFatal logLevel = "fatal"
)

func debugKV(component, event string, kv ...any) {
	if !debugMode {
		return
	}
	log.Print(formatLogLine(logLevelDebug, component, event, kv...))
}

func infoKV(component, event string, kv ...any) {
	log.Print(formatLogLine(logLevelInfo, component, event, kv...))
}

func warnKV(component, event string, kv ...any) {
	log.Print(formatLogLine(logLevelWarn, component, event, kv...))
}

func errorKV(component, event string, kv ...any) {
	log.Print(formatLogLine(logLevelError, component, event, kv...))
}

func fatalKV(component, event string, kv ...any) {
	log.Print(formatLogLine(logLevelFatal, component, event, kv...))
	os.Exit(1)
}

func configureLogOutput(path string) (*os.File, error) {
	if path == "" {
		log.SetOutput(os.Stderr)
		return nil, nil
	}

	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, err
	}

	log.SetOutput(io.MultiWriter(os.Stderr, file))
	return file, nil
}

func formatLogLine(level logLevel, component, event string, kv ...any) string {
	line := "level=" + string(level)
	line += " component=" + formatLogValue(component)
	line += " event=" + formatLogValue(event)

	for i := 0; i+1 < len(kv); i += 2 {
		key := formatLogKey(kv[i])
		line += " " + key + "=" + formatLogValue(kv[i+1])
	}

	if len(kv)%2 != 0 {
		line += " kv_error=" + formatLogValue("odd_argument_count")
		line += " orphan_value=" + formatLogValue(kv[len(kv)-1])
	}

	return line
}

func formatLogKey(v any) string {
	if s, ok := v.(string); ok && s != "" {
		return s
	}
	return "invalid_key"
}

func formatLogValue(v any) string {
	switch value := v.(type) {
	case nil:
		return "null"
	case string:
		return strconv.Quote(value)
	case error:
		return strconv.Quote(value.Error())
	case bool:
		return strconv.FormatBool(value)
	case int:
		return strconv.Itoa(value)
	case int8, int16, int32, int64:
		return fmt.Sprintf("%d", value)
	case uint, uint8, uint16, uint32, uint64:
		return fmt.Sprintf("%d", value)
	case float32:
		return strconv.FormatFloat(float64(value), 'f', -1, 32)
	case float64:
		return strconv.FormatFloat(value, 'f', -1, 64)
	case fmt.Stringer:
		return strconv.Quote(value.String())
	default:
		return strconv.Quote(fmt.Sprint(value))
	}
}
