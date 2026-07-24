package measure

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

const uploadChunkSize = 25 * 1024 * 1024 // 25MB per request

type uploadBody struct {
	remaining int64
	sent      *atomic.Int64
}

func (u *uploadBody) Read(p []byte) (int, error) {
	if u.remaining <= 0 {
		return 0, io.EOF
	}
	if int64(len(p)) > u.remaining {
		p = p[:u.remaining]
	}
	for i := range p {
		p[i] = 0
	}
	u.remaining -= int64(len(p))
	u.sent.Add(int64(len(p)))
	return len(p), nil
}

func Upload(targets []string, duration time.Duration, progressCh chan<- Progress, verboseCh chan<- VerboseLog, headers map[string]string, acceptableStatuses map[int]bool) error {
	if len(targets) == 0 {
		return fmt.Errorf("no targets provided")
	}

	log := func(msg string) {
		defer func() { recover() }()
		select {
		case verboseCh <- VerboseLog{Msg: msg}:
		default:
		}
	}

	var totalBytes atomic.Int64
	done := make(chan struct{})
	deadline := time.After(duration)

	var wg sync.WaitGroup
	for _, target := range targets {
		wg.Add(1)
		go func(url string) {
			defer wg.Done()
			for {
				select {
				case <-done:
					return
				default:
				}
				if err := uploadChunk(url, &totalBytes, done, log, headers, acceptableStatuses); err != nil {
					return
				}
			}
		}(target)
	}

	start := time.Now()
	ticker := time.NewTicker(sampleInterval)
	defer ticker.Stop()

	var lastBytes int64
	var lastTime time.Time
	var stableCount int

	for {
		select {
		case <-ticker.C:
			now := time.Now()
			elapsed := now.Sub(start).Seconds()
			currentBytes := totalBytes.Load()

			if lastTime.IsZero() {
				lastBytes = currentBytes
				lastTime = now
				continue
			}

			chunkElapsed := now.Sub(lastTime).Seconds()
			if chunkElapsed > 0 {
				chunkBytes := currentBytes - lastBytes
				instantMbps := float64(chunkBytes) * 8 / chunkElapsed / 1_000_000

				select {
				case progressCh <- Progress{
					Mbps:  instantMbps,
					Bytes: currentBytes,
				}:
				default:
				}

				log(fmt.Sprintf("sample: %.2f Mbps (%d bytes total, +%d bytes)", instantMbps, currentBytes, chunkBytes))

				if elapsed >= minTestDuration.Seconds() {
					var avgMbps float64
					if elapsed > 0 {
						avgMbps = float64(currentBytes) * 8 / elapsed / 1_000_000
					}

					if avgMbps > 0 {
						deviation := (instantMbps - avgMbps) / avgMbps
						if deviation < 0 {
							deviation = -deviation
						}
						if deviation < 0.05 {
							stableCount++
						} else {
							stableCount = 0
						}
					}

					if stableCount >= 15 {
						close(done)
						wg.Wait()
						progressCh <- Progress{
							Mbps:     float64(currentBytes) * 8 / elapsed / 1_000_000,
							Bytes:    currentBytes,
							Finished: true,
						}
						return nil
					}
				}
			}

			lastBytes = currentBytes
			lastTime = now

		case <-deadline:
			close(done)
			wg.Wait()
			finalBytes := totalBytes.Load()
			finalElapsed := time.Since(start).Seconds()
			finalMbps := float64(finalBytes) * 8 / finalElapsed / 1_000_000

			log(fmt.Sprintf("done: %.2f Mbps (%d bytes in %.1fs)", finalMbps, finalBytes, finalElapsed))
			progressCh <- Progress{
				Mbps:     finalMbps,
				Bytes:    finalBytes,
				Finished: true,
			}
			return nil
		}
	}
}

func uploadChunk(url string, totalBytes *atomic.Int64, done <-chan struct{}, log func(string), headers map[string]string, acceptableStatuses map[int]bool) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		<-done
		cancel()
	}()

	body := &uploadBody{
		remaining: uploadChunkSize,
		sent:      totalBytes,
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, body)
	if err != nil {
		log(fmt.Sprintf("request error: %v", err))
		return err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	req.Header.Set("Content-Type", "application/octet-stream")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log(fmt.Sprintf("http error: %v", err))
		return err
	}
	defer resp.Body.Close()

	if acceptableStatuses != nil && !acceptableStatuses[resp.StatusCode] {
		log(fmt.Sprintf("unexpected status %d from %s", resp.StatusCode, req.URL.Host))
		return fmt.Errorf("unexpected status: %d", resp.StatusCode)
	} else if acceptableStatuses == nil && (resp.StatusCode < 200 || resp.StatusCode >= 300) {
		log(fmt.Sprintf("unexpected status %d from %s", resp.StatusCode, req.URL.Host))
		return fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	log(fmt.Sprintf("connected to %s (status %d)", req.URL.Host, resp.StatusCode))

	buf := make([]byte, 32*1024)
	for {
		if _, readErr := resp.Body.Read(buf); readErr != nil {
			return nil
		}
	}
}
