package systemsvc

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	systemv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/system/v1"
)

type approvedLogs struct{}

func (approvedLogs) Read(source string, tailLines int, filter string) ([]*systemv1.LogLine, error) {
	path, err := logPath(source)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if tailLines <= 0 || tailLines > 200 {
		tailLines = 100
	}
	if len(lines) > tailLines {
		lines = lines[len(lines)-tailLines:]
	}
	filter = strings.ToLower(strings.TrimSpace(filter))
	out := make([]*systemv1.LogLine, 0, len(lines))
	for _, line := range lines {
		if filter == "" || strings.Contains(strings.ToLower(line), filter) {
			out = append(out, &systemv1.LogLine{Source: path, Message: line})
		}
	}
	return out, nil
}

func logPath(source string) (string, error) {
	switch strings.TrimSpace(source) {
	case "", "syslog":
		source = "/var/log/syslog"
	case "messages":
		source = "/var/log/messages"
	case "kern":
		source = "/var/log/kern.log"
	}
	if !filepath.IsAbs(source) {
		return "", fmt.Errorf("log source must be an approved absolute /var/log path")
	}
	clean := filepath.Clean(source)
	if clean != "/var/log" && !strings.HasPrefix(clean, "/var/log/") {
		return "", fmt.Errorf("log source must be under /var/log")
	}
	return clean, nil
}
