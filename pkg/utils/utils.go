package utils

import "os"

// GetEnv parses and defaults to fallback value if missing
func GetEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}
