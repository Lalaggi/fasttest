package mlab

import (
	"context"
	"fmt"
	"time"

	ndt7 "github.com/m-lab/ndt7-client-go"

	"codeberg.org/el1lovescomputers/fasttest/internal/backend"
	"codeberg.org/el1lovescomputers/fasttest/internal/measure"
)

type Backend struct{}

func New() *Backend { return &Backend{} }

func (b *Backend) Name() string { return "mlab" }

func (b *Backend) Init(_ context.Context) (backend.ClientInfo, error) {
	return backend.ClientInfo{}, nil
}

func (b *Backend) DiscoverServers(_ context.Context, count int) ([]backend.Server, error) {
	c := ndt7.NewClient("fasttest", "1.0.0")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ch, err := c.StartDownload(ctx)
	if err != nil {
		return nil, fmt.Errorf("server discovery failed: %w", err)
	}
	for range ch {
		break
	}

	if c.FQDN == "" {
		return nil, fmt.Errorf("no server discovered")
	}

	servers := make([]backend.Server, count)
	for i := 0; i < count; i++ {
		servers[i] = backend.Server{
			ID:       c.FQDN,
			Name:     c.FQDN,
			Location: "nearest",
			URL:      c.FQDN,
		}
	}
	return servers, nil
}

func (b *Backend) TestDownload(_ context.Context, servers []backend.Server, opts backend.TestOpts, progress chan<- measure.Progress, verboseCh chan<- measure.VerboseLog) error {
	log := func(msg string) {
		select {
		case verboseCh <- measure.VerboseLog{Msg: msg}:
		default:
		}
	}
	defer close(verboseCh)

	c := ndt7.NewClient("fasttest", "1.0.0")
	if len(servers) > 0 {
		c.Server = servers[0].URL
	}

	ctx, cancel := context.WithTimeout(context.Background(), opts.Duration+10*time.Second)
	defer cancel()

	log("starting ndt7 download test...")
	ch, err := c.StartDownload(ctx)
	if err != nil {
		return fmt.Errorf("starting download: %w", err)
	}

	start := time.Now()
	var totalBytes int64
	var lastBytes int64
	lastTime := start
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case m, ok := <-ch:
			if !ok {
				elapsed := time.Since(start).Seconds()
				mbps := float64(totalBytes) * 8 / elapsed / 1_000_000
				progress <- measure.Progress{Mbps: mbps, Bytes: totalBytes, Finished: true}
				return nil
			}
			if m.AppInfo != nil {
				totalBytes = m.AppInfo.NumBytes
			}
		case now := <-ticker.C:
			chunkBytes := totalBytes - lastBytes
			chunkElapsed := now.Sub(lastTime).Seconds()
			if chunkElapsed > 0 {
				instantMbps := float64(chunkBytes) * 8 / chunkElapsed / 1_000_000
				select {
				case progress <- measure.Progress{Mbps: instantMbps, Bytes: totalBytes}:
				default:
				}
				log(fmt.Sprintf("sample: %.2f Mbps (%d bytes)", instantMbps, totalBytes))
			}
			lastBytes = totalBytes
			lastTime = now
		}
	}
}

func (b *Backend) TestUpload(_ context.Context, servers []backend.Server, opts backend.TestOpts, progress chan<- measure.Progress, verboseCh chan<- measure.VerboseLog) error {
	log := func(msg string) {
		select {
		case verboseCh <- measure.VerboseLog{Msg: msg}:
		default:
		}
	}
	defer close(verboseCh)

	c := ndt7.NewClient("fasttest", "1.0.0")
	if len(servers) > 0 {
		c.Server = servers[0].URL
	}

	ctx, cancel := context.WithTimeout(context.Background(), opts.Duration+10*time.Second)
	defer cancel()

	log("starting ndt7 upload test...")
	ch, err := c.StartUpload(ctx)
	if err != nil {
		return fmt.Errorf("starting upload: %w", err)
	}

	start := time.Now()
	var totalBytes int64
	var lastBytes int64
	lastTime := start
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case m, ok := <-ch:
			if !ok {
				elapsed := time.Since(start).Seconds()
				mbps := float64(totalBytes) * 8 / elapsed / 1_000_000
				progress <- measure.Progress{Mbps: mbps, Bytes: totalBytes, Finished: true}
				return nil
			}
			if m.AppInfo != nil {
				totalBytes = m.AppInfo.NumBytes
			}
		case now := <-ticker.C:
			chunkBytes := totalBytes - lastBytes
			chunkElapsed := now.Sub(lastTime).Seconds()
			if chunkElapsed > 0 {
				instantMbps := float64(chunkBytes) * 8 / chunkElapsed / 1_000_000
				select {
				case progress <- measure.Progress{Mbps: instantMbps, Bytes: totalBytes}:
				default:
				}
				log(fmt.Sprintf("sample: %.2f Mbps (%d bytes)", instantMbps, totalBytes))
			}
			lastBytes = totalBytes
			lastTime = now
		}
	}
}

func (b *Backend) TestLatency(_ context.Context, server backend.Server) (time.Duration, error) {
	c := ndt7.NewClient("fasttest", "1.0.0")
	c.Server = server.URL

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ch, err := c.StartDownload(ctx)
	if err != nil {
		return 0, err
	}

	for m := range ch {
		if m.TCPInfo != nil {
			rtt := m.TCPInfo.RTT
			if rtt > 0 {
				return time.Duration(rtt) * time.Microsecond, nil
			}
		}
	}

	return 0, fmt.Errorf("no latency measurement received")
}
