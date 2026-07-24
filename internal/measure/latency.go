package measure

import (
	"fmt"
	"net/http"
	"sort"
	"time"
)

func Latency(targetURL string) (time.Duration, error) {
	const samples = 5
	latencies := make([]time.Duration, 0, samples)

	for i := 0; i < samples; i++ {
		rtt, err := measureSingleLatency(targetURL)
		if err != nil {
			return 0, err
		}
		latencies = append(latencies, rtt)
	}

	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	return latencies[len(latencies)/2], nil
}

func measureSingleLatency(targetURL string) (time.Duration, error) {
	req, err := http.NewRequest("GET", targetURL, nil)
	if err != nil {
		return 0, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Range", "bytes=0-0")
	req.Header.Set("Referer", "https://fast.com/")
	req.Header.Set("Origin", "https://fast.com")
	req.Header.Set("Accept-Encoding", "identity")

	start := time.Now()
	resp, err := http.DefaultClient.Do(req)
	elapsed := time.Since(start)

	if err != nil {
		return 0, fmt.Errorf("latency request failed: %w", err)
	}
	defer resp.Body.Close()

	return elapsed, nil
}
