package fastly

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"codeberg.org/el1lovescomputers/fasttest/internal/backend"
	"codeberg.org/el1lovescomputers/fasttest/internal/measure"
)

const (
	downloadURL = "https://speedtest.fastly.com/__down?bytes=25000000"
	uploadURL   = "https://speedtest.fastly.com/__up?bytes=25000000"
	traceURL    = "https://speedtest.fastly.com/cdn-cgi/trace"
)

var headers = map[string]string{
	"User-Agent":      "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
	"Accept":          "*/*",
	"Accept-Encoding": "identity",
	"Referer":         "https://speedtest.fastly.com/",
}

type Backend struct{}

func New() *Backend { return &Backend{} }

func (b *Backend) Name() string { return "fastly" }

func (b *Backend) Init(_ context.Context) (backend.ClientInfo, error) {
	req, err := http.NewRequest("GET", traceURL, nil)
	if err != nil {
		return backend.ClientInfo{}, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return backend.ClientInfo{}, fmt.Errorf("fetching trace: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return backend.ClientInfo{}, fmt.Errorf("trace returned status %d", resp.StatusCode)
	}

	info := parseTrace(resp)
	loc := ""
	if info["city"] != "" {
		loc = info["city"] + ", " + info["country"]
	}

	return backend.ClientInfo{
		IP:       info["ip"],
		ASN:      info["asn"],
		Location: loc,
	}, nil
}

func parseTrace(resp *http.Response) map[string]string {
	info := make(map[string]string)
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if idx := strings.IndexByte(line, '='); idx > 0 {
			info[strings.TrimSpace(line[:idx])] = strings.TrimSpace(line[idx+1:])
		}
	}
	return info
}

func (b *Backend) DiscoverServers(_ context.Context, count int) ([]backend.Server, error) {
	servers := make([]backend.Server, count)
	for i := 0; i < count; i++ {
		servers[i] = backend.Server{
			ID:       fmt.Sprintf("fastly-%d", i),
			Name:     "Fastly Edge",
			Location: "nearest",
			URL:      downloadURL,
		}
	}
	return servers, nil
}

func (b *Backend) TestDownload(_ context.Context, servers []backend.Server, opts backend.TestOpts, progress chan<- measure.Progress, verboseCh chan<- measure.VerboseLog) error {
	targets := make([]string, len(servers))
	for i, s := range servers {
		targets[i] = s.URL
	}
	return measure.Download(targets, opts.Duration, progress, verboseCh, headers)
}

func (b *Backend) TestUpload(_ context.Context, servers []backend.Server, opts backend.TestOpts, progress chan<- measure.Progress, verboseCh chan<- measure.VerboseLog) error {
	targets := make([]string, len(servers))
	for i := range servers {
		targets[i] = uploadURL
	}
	return measure.Upload(targets, opts.Duration, progress, verboseCh, headers, nil)
}

func (b *Backend) TestLatency(_ context.Context, _ backend.Server) (time.Duration, error) {
	const samples = 5
	latencies := make([]time.Duration, 0, samples)

	for i := 0; i < samples; i++ {
		req, err := http.NewRequest("GET", traceURL, nil)
		if err != nil {
			return 0, err
		}
		for k, v := range headers {
			req.Header.Set(k, v)
		}

		start := time.Now()
		resp, err := http.DefaultClient.Do(req)
		elapsed := time.Since(start)
		if err != nil {
			return 0, err
		}
		resp.Body.Close()
		latencies = append(latencies, elapsed)
	}

	sortDurations(latencies)
	return latencies[len(latencies)/2], nil
}

func sortDurations(d []time.Duration) {
	for i := 1; i < len(d); i++ {
		for j := i; j > 0 && d[j] < d[j-1]; j-- {
			d[j], d[j-1] = d[j-1], d[j]
		}
	}
}
