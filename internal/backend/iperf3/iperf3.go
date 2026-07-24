package iperf3

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"codeberg.org/el1lovescomputers/fasttest/internal/backend"
	"codeberg.org/el1lovescomputers/fasttest/internal/measure"
)

const (
	serverListURL = "https://iperf3serverlist.net/api/servers"
	traceURL      = "https://speed.cloudflare.com/cdn-cgi/trace"
	defaultPort   = 5201
)

var headers = map[string]string{
	"User-Agent": "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
}

type apiServer struct {
	IP       string `json:"ip"`
	Port     string `json:"port"`
	Country  string `json:"country"`
	Location string `json:"location"`
	Provider string `json:"provider"`
}

type curatedServer struct {
	Hosts    []string `json:"iperf3_server"`
	Loc      string   `json:"localization"`
	Hosting  string   `json:"hosting"`
	Port     string   `json:"port"`
	TestDate string   `json:"test_date"`
}

type curatedData struct {
	Europe   []curatedServer `json:"Europe"`
	Africa   []curatedServer `json:"Africa"`
	Asia     []curatedServer `json:"Asia"`
	Oceania  []curatedServer `json:"Oceania"`
	Americas []curatedServer `json:"Americas"`
}

type iperfReport struct {
	End struct {
		SumSent struct {
			BitsPerSecond float64 `json:"bits_per_second"`
		} `json:"sum_sent"`
		SumReceived struct {
			BitsPerSecond float64 `json:"bits_per_second"`
		} `json:"sum_received"`
	} `json:"end"`
	Error string `json:"error"`
}

type Backend struct{}

func New() *Backend {
	return &Backend{}
}

func (b *Backend) Name() string {
	return "iperf3"
}

func (b *Backend) Init(_ context.Context) (backend.ClientInfo, error) {
	req, err := http.NewRequest("GET", traceURL, nil)
	if err != nil {
		return backend.ClientInfo{}, fmt.Errorf("creating trace request: %w", err)
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
	if info["loc"] != "" {
		loc = info["loc"]
	} else if info["city"] != "" {
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
			key := strings.TrimSpace(line[:idx])
			val := strings.TrimSpace(line[idx+1:])
			info[key] = val
		}
	}
	return info
}

func (b *Backend) DiscoverServers(_ context.Context, count int) ([]backend.Server, error) {
	entries, err := fetchServers()
	if err != nil {
		return nil, err
	}

	if len(entries) == 0 {
		return nil, fmt.Errorf("no servers available")
	}

	servers := make([]backend.Server, 0, len(entries))
	for _, e := range entries {
		port := parseFirstPort(e.Port)
		if port == 0 {
			continue
		}
		loc := e.Location
		if e.Country != "" {
			loc = e.Location + ", " + e.Country
		}
		servers = append(servers, backend.Server{
			ID:       e.IP,
			Name:     e.Provider,
			Location: loc,
			URL:      strconv.Itoa(port),
		})
	}

	if len(servers) == 0 {
		return nil, fmt.Errorf("no usable servers found")
	}

	sortByLatency(servers)

	if count > len(servers) {
		count = len(servers)
	}
	return servers[:count], nil
}

func fetchServers() ([]apiServer, error) {
	var combined []apiServer

	resp, err := http.Get(serverListURL)
	if err == nil {
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			var entries []apiServer
			if json.NewDecoder(resp.Body).Decode(&entries) == nil {
				combined = append(combined, entries...)
			}
		}
	}

	curated, err := fetchCuratedServers()
	if err == nil {
		combined = append(combined, curated...)
	}

	deduped := deduplicate(combined)
	rand.Shuffle(len(deduped), func(i, j int) {
		deduped[i], deduped[j] = deduped[j], deduped[i]
	})

	return deduped, nil
}

func fetchCuratedServers() ([]apiServer, error) {
	urls := []string{
		"https://iperf.fr/iperf-servers.json",
	}

	var all []apiServer
	for _, url := range urls {
		resp, err := http.Get(url)
		if err != nil {
			continue
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			continue
		}

		var data curatedData
		if json.NewDecoder(resp.Body).Decode(&data) != nil {
			continue
		}

		allServers := append(append(append(append(data.Europe, data.Africa...), data.Asia...), data.Oceania...), data.Americas...)
		for _, s := range allServers {
			port := parseFirstPort(s.Port)
			if port == 0 {
				port = defaultPort
			}
			for _, host := range s.Hosts {
				if host == "" {
					continue
				}
				all = append(all, apiServer{
					IP:       host,
					Port:     strconv.Itoa(port),
					Country:  extractCountry(s.Loc),
					Location: s.Loc,
					Provider: s.Hosting,
				})
			}
		}
	}
	return all, nil
}

func extractCountry(loc string) string {
	parts := strings.SplitN(loc, ",", 2)
	if len(parts) > 0 {
		return strings.TrimSpace(parts[0])
	}
	return loc
}

func deduplicate(entries []apiServer) []apiServer {
	seen := make(map[string]bool)
	var result []apiServer
	for _, e := range entries {
		key := e.IP
		if seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, e)
	}
	return result
}

func parseFirstPort(portStr string) int {
	if portStr == "" {
		return defaultPort
	}
	if idx := strings.IndexByte(portStr, '-'); idx > 0 {
		portStr = portStr[:idx]
	}
	port, err := strconv.Atoi(strings.TrimSpace(portStr))
	if err != nil {
		return defaultPort
	}
	return port
}

func sortByLatency(servers []backend.Server) {
	type scored struct {
		server backend.Server
		lat    time.Duration
	}

	items := make([]scored, len(servers))
	var mu sync.Mutex
	var wg sync.WaitGroup

	for i, s := range servers {
		wg.Add(1)
		go func(idx int, srv backend.Server) {
			defer wg.Done()
			port := srv.URL
			addr := net.JoinHostPort(srv.ID, port)
			lat := measureTCPConn(addr)
			mu.Lock()
			items[idx] = scored{server: srv, lat: lat}
			mu.Unlock()
		}(i, s)
	}
	wg.Wait()

	sort.Slice(items, func(i, j int) bool {
		if items[i].lat == 0 {
			return false
		}
		if items[j].lat == 0 {
			return true
		}
		return items[i].lat < items[j].lat
	})

	for i, item := range items {
		servers[i] = item.server
	}
}

func measureTCPConn(addr string) time.Duration {
	conn, err := net.DialTimeout("tcp", addr, 3*time.Second)
	if err != nil {
		return 0
	}
	conn.Close()

	start := time.Now()
	conn, err = net.DialTimeout("tcp", addr, 3*time.Second)
	elapsed := time.Since(start)
	if err != nil {
		return 0
	}
	conn.Close()
	return elapsed
}

func runIperf3(ctx context.Context, host, port string, reverse bool, duration int, progress chan<- measure.Progress, verboseCh chan<- measure.VerboseLog) (float64, error) {
	args := []string{
		"-oL",
		"iperf3",
		"-c", host,
		"-p", port,
		"-t", strconv.Itoa(duration),
		"--connect-timeout", "5000",
		"--rcv-timeout", "10000",
	}
	if reverse {
		args = append(args, "-R")
	}

	cmd := exec.CommandContext(ctx, "stdbuf", args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return 0, fmt.Errorf("creating pipe: %w", err)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return 0, fmt.Errorf("starting iperf3: %w", err)
	}

	var lastMbps float64
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := scanner.Text()
		mbps := parseIntervalLine(line)
		if mbps > 0 {
			lastMbps = mbps
			progress <- measure.Progress{Mbps: mbps}
		}
	}

	if err := cmd.Wait(); err != nil {
		msg := stderr.String()
		if msg == "" {
			msg = err.Error()
		}
		return 0, fmt.Errorf("iperf3: %s", msg)
	}

	return lastMbps, nil
}

func parseIntervalLine(line string) float64 {
	if !strings.Contains(line, "Mbits/sec") && !strings.Contains(line, "Gbits/sec") {
		return 0
	}
	if strings.Contains(line, "sender") || strings.Contains(line, "receiver") {
		parts := strings.Fields(line)
		for i, p := range parts {
			if p == "Mbits/sec" && i > 0 {
				val, err := strconv.ParseFloat(parts[i-1], 64)
				if err == nil {
					return val
				}
			}
			if p == "Gbits/sec" && i > 0 {
				val, err := strconv.ParseFloat(parts[i-1], 64)
				if err == nil {
					return val * 1000
				}
			}
		}
		return 0
	}
	parts := strings.Fields(line)
	for i, p := range parts {
		if p == "Mbits/sec" && i > 0 {
			val, err := strconv.ParseFloat(parts[i-1], 64)
			if err == nil {
				return val
			}
		}
		if p == "Gbits/sec" && i > 0 {
			val, err := strconv.ParseFloat(parts[i-1], 64)
			if err == nil {
				return val * 1000
			}
		}
	}
	return 0
}

func (b *Backend) TestDownload(ctx context.Context, servers []backend.Server, opts backend.TestOpts, progress chan<- measure.Progress, verboseCh chan<- measure.VerboseLog) error {
	if len(servers) == 0 {
		return fmt.Errorf("no servers available")
	}

	secs := int(opts.Duration.Seconds())
	var lastErr error

	for i, srv := range servers {
		verboseCh <- measure.VerboseLog{Msg: fmt.Sprintf("trying server %d/%d: %s:%s (%s)", i+1, len(servers), srv.ID, srv.URL, srv.Location)}

		mbps, err := runIperf3(ctx, srv.ID, srv.URL, true, secs, progress, verboseCh)
		if err != nil {
			lastErr = err
			verboseCh <- measure.VerboseLog{Msg: fmt.Sprintf("  failed: %v", err)}
			continue
		}
		if mbps == 0 {
			lastErr = fmt.Errorf("zero throughput on %s", srv.ID)
			verboseCh <- measure.VerboseLog{Msg: fmt.Sprintf("  failed: %v", lastErr)}
			continue
		}

		verboseCh <- measure.VerboseLog{Msg: fmt.Sprintf("download: %.2f Mbps (from %s)", mbps, srv.ID)}
		progress <- measure.Progress{Mbps: mbps, Finished: true}
		return nil
	}

	return fmt.Errorf("all servers failed, last error: %w", lastErr)
}

func (b *Backend) TestUpload(ctx context.Context, servers []backend.Server, opts backend.TestOpts, progress chan<- measure.Progress, verboseCh chan<- measure.VerboseLog) error {
	if len(servers) == 0 {
		return fmt.Errorf("no servers available")
	}

	secs := int(opts.Duration.Seconds())
	var lastErr error

	for i, srv := range servers {
		verboseCh <- measure.VerboseLog{Msg: fmt.Sprintf("trying server %d/%d: %s:%s (%s)", i+1, len(servers), srv.ID, srv.URL, srv.Location)}

		mbps, err := runIperf3(ctx, srv.ID, srv.URL, false, secs, progress, verboseCh)
		if err != nil {
			lastErr = err
			verboseCh <- measure.VerboseLog{Msg: fmt.Sprintf("  failed: %v", err)}
			continue
		}
		if mbps == 0 {
			lastErr = fmt.Errorf("zero throughput on %s", srv.ID)
			verboseCh <- measure.VerboseLog{Msg: fmt.Sprintf("  failed: %v", lastErr)}
			continue
		}

		verboseCh <- measure.VerboseLog{Msg: fmt.Sprintf("upload: %.2f Mbps (from %s)", mbps, srv.ID)}
		progress <- measure.Progress{Mbps: mbps, Finished: true}
		return nil
	}

	return fmt.Errorf("all servers failed, last error: %w", lastErr)
}

func (b *Backend) TestLatency(_ context.Context, server backend.Server) (time.Duration, error) {
	addr := net.JoinHostPort(server.ID, server.URL)
	lat := measureTCPConn(addr)
	if lat == 0 {
		return 0, fmt.Errorf("could not reach %s", addr)
	}
	return lat, nil
}
