package internetspeedtest

import (
	"context"
	"time"

	"github.com/Lalaggi/fasttest/internal/backend"
	"github.com/Lalaggi/fasttest/internal/backend/librespeed"
	"github.com/Lalaggi/fasttest/internal/measure"
)

const serverListURL = "https://internetspeedtest.net/api/servers"

type Backend struct {
	*librespeed.Backend
}

func New() *Backend {
	return &Backend{Backend: librespeed.NewWithServerList(serverListURL)}
}

func (b *Backend) Name() string { return "internetspeedtest" }

func (b *Backend) TestLatency(ctx context.Context, server backend.Server) (time.Duration, error) {
	return b.Backend.TestLatency(ctx, server)
}

func (b *Backend) TestDownload(ctx context.Context, servers []backend.Server, opts backend.TestOpts, progress chan<- measure.Progress, verboseCh chan<- measure.VerboseLog) error {
	return b.Backend.TestDownload(ctx, servers, opts, progress, verboseCh)
}

func (b *Backend) TestUpload(ctx context.Context, servers []backend.Server, opts backend.TestOpts, progress chan<- measure.Progress, verboseCh chan<- measure.VerboseLog) error {
	return b.Backend.TestUpload(ctx, servers, opts, progress, verboseCh)
}
