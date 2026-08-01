package deployment

import "strings"

const DefaultLogLevel = "info"

var validLogLevels = map[string]struct{}{
	"quiet":  {},
	"silent": {},
	"info":   {},
	"debug":  {},
}

// NormalizeLogLevel returns a supported log level, defaulting to info.
func NormalizeLogLevel(level string) string {
	level = strings.ToLower(strings.TrimSpace(level))
	if level == "" {
		return DefaultLogLevel
	}
	if _, ok := validLogLevels[level]; ok {
		return level
	}
	return DefaultLogLevel
}
