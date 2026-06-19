package cli

import "strings"

// logFormat returns the log format based on the environment.
// local → "text" (colored), all others → "json".
func logFormat(environment string) string {
	if strings.EqualFold(environment, "local") {
		return "text"
	}
	return "json"
}
