package util

import (
	"bufio"
	"net/http"
	"strings"
)

func ParseTrace(resp *http.Response) map[string]string {
	info := make(map[string]string)
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if idx := strings.IndexByte(line, '='); idx > 0 {
			key := strings.TrimSpace(line[:idx])
			val := strings.TrimSpace(line[idx+1:])
			info[key] = val
		}
	}
	return info
}
