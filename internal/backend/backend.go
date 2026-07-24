package backend

import (
	"context"
	"time"

	"codeberg.org/el1lovescomputers/fasttest/internal/measure"
)

type Server struct {
	ID       string
	Name     string
	Location string
	URL      string
}

type ClientInfo struct {
	IP       string
	ASN      string
	Location string
}

type TestOpts struct {
	Duration    time.Duration
	ServerCount int
}

type Backend interface {
	Name() string
	Init(ctx context.Context) (ClientInfo, error)
	DiscoverServers(ctx context.Context, count int) ([]Server, error)
	TestDownload(ctx context.Context, servers []Server, opts TestOpts, progress chan<- measure.Progress, verboseCh chan<- measure.VerboseLog) error
	TestUpload(ctx context.Context, servers []Server, opts TestOpts, progress chan<- measure.Progress, verboseCh chan<- measure.VerboseLog) error
	TestLatency(ctx context.Context, server Server) (time.Duration, error)
}
