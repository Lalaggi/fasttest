package fastcom

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/Lalaggi/fasttest/internal/backend"
	"github.com/Lalaggi/fasttest/internal/measure"
)

type Backend struct {
	token    string
	clientIP string
	asn      string
	location string
}

func New() *Backend {
	return &Backend{}
}

func (b *Backend) Name() string {
	return "fastcom"
}

func (b *Backend) Init(_ context.Context) (backend.ClientInfo, error) {
	tk, err := getToken()
	if err != nil {
		return backend.ClientInfo{}, err
	}
	b.token = tk

	ci, targets, err := getTargets(tk, 1)
	if err != nil {
		return backend.ClientInfo{}, err
	}

	b.clientIP = ci.IP
	b.asn = ci.ASN
	if ci.Location.City != "" {
		b.location = ci.Location.City + ", " + ci.Location.Country
	}

	loc := b.location
	_ = targets

	return backend.ClientInfo{
		IP:       b.clientIP,
		ASN:      b.asn,
		Location: loc,
	}, nil
}

func (b *Backend) DiscoverServers(_ context.Context, count int) ([]backend.Server, error) {
	_, targets, err := getTargets(b.token, count)
	if err != nil {
		return nil, err
	}

	servers := make([]backend.Server, len(targets))
	for i, t := range targets {
		loc := ""
		if t.Location.City != "" {
			loc = t.Location.City + ", " + t.Location.Country
		}
		servers[i] = backend.Server{
			ID:       fmt.Sprintf("fast-%d", i),
			Name:     "Netflix OCA",
			Location: loc,
			URL:      t.URL,
		}
	}
	return servers, nil
}

var fastHeaders = map[string]string{
	"Referer":       "https://fast.com/",
	"Origin":        "https://fast.com",
	"Accept-Encoding": "identity",
}

func (b *Backend) TestDownload(_ context.Context, servers []backend.Server, opts backend.TestOpts, progress chan<- measure.Progress, verboseCh chan<- measure.VerboseLog) error {
	targets := make([]string, len(servers))
	for i, s := range servers {
		targets[i] = s.URL
	}
	return measure.Download(targets, opts.Duration, progress, verboseCh, fastHeaders)
}

func (b *Backend) TestUpload(_ context.Context, servers []backend.Server, opts backend.TestOpts, progress chan<- measure.Progress, verboseCh chan<- measure.VerboseLog) error {
	targets := make([]string, len(servers))
	for i, s := range servers {
		targets[i] = s.URL
	}
	// fast.com servers return 406 on upload (accepted, not an error)
	acceptable := map[int]bool{200: true, 406: true}
	return measure.Upload(targets, opts.Duration, progress, verboseCh, fastHeaders, acceptable)
}

func (b *Backend) TestLatency(_ context.Context, server backend.Server) (time.Duration, error) {
	const samples = 5
	latencies := make([]time.Duration, 0, samples)

	for i := 0; i < samples; i++ {
		req, err := http.NewRequest("GET", server.URL, nil)
		if err != nil {
			return 0, err
		}
		req.Header.Set("Range", "bytes=0-0")
		req.Header.Set("Referer", "https://fast.com/")
		req.Header.Set("Origin", "https://fast.com")
		req.Header.Set("Accept-Encoding", "identity")

		start := time.Now()
		resp, err := http.DefaultClient.Do(req)
		elapsed := time.Since(start)

		if err != nil {
			return 0, err
		}
		resp.Body.Close()

		latencies = append(latencies, elapsed)
	}

	for i := 1; i < len(latencies); i++ {
		for j := i; j > 0 && latencies[j] < latencies[j-1]; j-- {
			latencies[j], latencies[j-1] = latencies[j-1], latencies[j]
		}
	}
	return latencies[len(latencies)/2], nil
}
