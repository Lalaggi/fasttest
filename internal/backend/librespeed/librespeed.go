package librespeed

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"codeberg.org/el1lovescomputers/fasttest/internal/backend"
	"codeberg.org/el1lovescomputers/fasttest/internal/measure"
)

const defaultServerList = "https://librespeed.org/backend-servers/servers.php"

var defaultHeaders = map[string]string{
	"User-Agent":      "fasttest/1.0",
	"Accept-Encoding": "identity",
}

type serverJSON struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Server   string `json:"server"`
	DlURL    string `json:"dlURL"`
	UlURL    string `json:"ulURL"`
	PingURL  string `json:"pingURL"`
	GetIPURL string `json:"getIpURL"`
	Sponsor  string `json:"sponsorName"`
}

type Backend struct {
	serverListURL string
	servers       []serverJSON
}

func New() *Backend {
	return &Backend{serverListURL: defaultServerList}
}

func NewWithServerList(url string) *Backend {
	return &Backend{serverListURL: url}
}

func (b *Backend) Name() string { return "librespeed" }

func (b *Backend) Init(ctx context.Context) (backend.ClientInfo, error) {
	servers, err := b.fetchServers(ctx)
	if err != nil {
		return backend.ClientInfo{}, err
	}
	b.servers = servers

	if len(servers) == 0 {
		return backend.ClientInfo{}, fmt.Errorf("no servers available")
	}

	// get IP info from the first server
	s := servers[0]
	getIPURL := s.Server + s.GetIPURL
	req, err := http.NewRequestWithContext(ctx, "GET", getIPURL, nil)
	if err != nil {
		return backend.ClientInfo{}, err
	}
	for k, v := range defaultHeaders {
		req.Header.Set(k, v)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return backend.ClientInfo{}, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return backend.ClientInfo{}, err
	}

	// Try to parse as JSON first, fall back to plain text
	var ipInfo struct {
		IP       string `json:"ip"`
		ASN      string `json:"asn"`
		Location string `json:"location"`
	}
	if err := json.Unmarshal(body, &ipInfo); err == nil {
		return backend.ClientInfo{
			IP:       ipInfo.IP,
			ASN:      ipInfo.ASN,
			Location: ipInfo.Location,
		}, nil
	}

	return backend.ClientInfo{IP: string(body)}, nil
}

func (b *Backend) fetchServers(ctx context.Context) ([]serverJSON, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", b.serverListURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "fasttest/1.0")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching server list: %w", err)
	}
	defer resp.Body.Close()

	var servers []serverJSON
	if err := json.NewDecoder(resp.Body).Decode(&servers); err != nil {
		return nil, fmt.Errorf("parsing server list: %w", err)
	}

	return servers, nil
}

func (b *Backend) DiscoverServers(_ context.Context, count int) ([]backend.Server, error) {
	if len(b.servers) == 0 {
		return nil, fmt.Errorf("no servers discovered yet")
	}

	servers := make([]backend.Server, 0, count)
	for i := 0; i < len(b.servers) && i < count; i++ {
		s := b.servers[i]
		servers = append(servers, backend.Server{
			ID:       fmt.Sprintf("ls-%d", s.ID),
			Name:     s.Name,
			Location: s.Name,
			URL:      s.Server,
		})
	}
	return servers, nil
}

func (b *Backend) TestDownload(_ context.Context, servers []backend.Server, opts backend.TestOpts, progress chan<- measure.Progress, verboseCh chan<- measure.VerboseLog) error {
	targets := make([]string, len(servers))
	for i, s := range servers {
		// find the matching server to get dlURL
		for _, sv := range b.servers {
			if s.URL == sv.Server {
				targets[i] = sv.Server + sv.DlURL
				break
			}
		}
		if targets[i] == "" {
			targets[i] = s.URL + "garbage"
		}
	}
	return measure.Download(targets, opts.Duration, progress, verboseCh, defaultHeaders)
}

func (b *Backend) TestUpload(_ context.Context, servers []backend.Server, opts backend.TestOpts, progress chan<- measure.Progress, verboseCh chan<- measure.VerboseLog) error {
	targets := make([]string, len(servers))
	for i, s := range servers {
		for _, sv := range b.servers {
			if s.URL == sv.Server {
				targets[i] = sv.Server + sv.UlURL
				break
			}
		}
		if targets[i] == "" {
			targets[i] = s.URL + "empty"
		}
	}
	return measure.Upload(targets, opts.Duration, progress, verboseCh, defaultHeaders, nil)
}

func (b *Backend) TestLatency(ctx context.Context, server backend.Server) (time.Duration, error) {
	pingURL := server.URL
	for _, sv := range b.servers {
		if server.URL == sv.Server {
			pingURL = sv.Server + sv.PingURL
			break
		}
	}

	const samples = 5
	latencies := make([]time.Duration, 0, samples)

	for i := 0; i < samples; i++ {
		req, err := http.NewRequestWithContext(ctx, "GET", pingURL, nil)
		if err != nil {
			return 0, err
		}
		for k, v := range defaultHeaders {
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
