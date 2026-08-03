package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	"github.com/Lalaggi/fasttest/internal/backend"
	"github.com/Lalaggi/fasttest/internal/backend/cloudflare"
	"github.com/Lalaggi/fasttest/internal/backend/fastcom"
	"github.com/Lalaggi/fasttest/internal/backend/internetspeedtest"
	iperf3backend "github.com/Lalaggi/fasttest/internal/backend/iperf3"
	"github.com/Lalaggi/fasttest/internal/backend/librespeed"
	"github.com/Lalaggi/fasttest/internal/measure"
	"github.com/Lalaggi/fasttest/internal/result"
)

var (
	downloadOnly bool
	uploadOnly   bool
	pingOnly     bool
	jsonOutput   bool
	verbose      bool
	duration     time.Duration
	servers      int
	backendName  string
)

var rootCmd = &cobra.Command{
	Use:   "fasttest",
	Short: "Internet speed test",
	Long:  "Measures download, upload, and latency using various backends.",
	RunE:  run,
}

func init() {
	rootCmd.Flags().BoolVar(&downloadOnly, "download-only", false, "measure download speed only")
	rootCmd.Flags().BoolVar(&uploadOnly, "upload-only", false, "measure upload speed only")
	rootCmd.Flags().BoolVar(&pingOnly, "ping-only", false, "measure latency only")
	rootCmd.Flags().BoolVar(&jsonOutput, "json", false, "output results as JSON")
	rootCmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "show detailed progress output")
	rootCmd.Flags().DurationVar(&duration, "duration", 20*time.Second, "test duration")
	rootCmd.Flags().IntVarP(&servers, "servers", "s", 5, "number of test servers")
	rootCmd.Flags().StringVarP(&backendName, "backend", "b", "cloudflare", "backend (cloudflare, fastcom, internetspeedtest, iperf3, librespeed)")
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func getBackend(name string) (backend.Backend, error) {
	switch name {
	case "cloudflare":
		return cloudflare.New(), nil
	case "fastcom":
		return fastcom.New(), nil
	case "internetspeedtest":
		return internetspeedtest.New(), nil
	case "iperf3":
		return iperf3backend.New(), nil
	case "librespeed":
		return librespeed.New(), nil
	default:
		return nil, fmt.Errorf("unknown backend: %s\navailable backends: cloudflare, fastcom, internetspeedtest, iperf3, librespeed", name)
	}
}

func run(cmd *cobra.Command, args []string) error {
	b, err := getBackend(backendName)
	if err != nil {
		return err
	}

	ctx := context.Background()

	fmt.Printf("Using %s backend...\n", b.Name())
	if backendName == "iperf3" {
		fmt.Fprintln(os.Stderr, "Warning: many public iperf3 servers reject upload tests. Upload may fail.")
	}
	fmt.Println("Initializing...")
	clientInfo, err := b.Init(ctx)
	if err != nil {
		return fmt.Errorf("init error: %w", err)
	}
	if verbose {
		fmt.Fprintf(os.Stderr, "  client: %s (ASN %s, %s)\n", clientInfo.IP, clientInfo.ASN, clientInfo.Location)
	}

	fmt.Println("Discovering servers...")
	serverList, err := b.DiscoverServers(ctx, servers)
	if err != nil {
		return fmt.Errorf("server discovery error: %w", err)
	}
	if verbose {
		for i, s := range serverList {
			fmt.Fprintf(os.Stderr, "  server %d: %s (%s)\n", i+1, s.Name, s.Location)
		}
	}

	res := result.Result{
		ServerIP: clientInfo.IP,
		ASN:      clientInfo.ASN,
		Location: clientInfo.Location,
	}

	opts := backend.TestOpts{
		Duration:    duration,
		ServerCount: servers,
	}

	runDownload := !uploadOnly && !pingOnly
	runUpload := !downloadOnly && !pingOnly
	runPing := !downloadOnly && !uploadOnly

	if runPing {
		if verbose {
			fmt.Fprintln(os.Stderr, "\n--- Ping ---")
			lat, err := b.TestLatency(ctx, serverList[0])
			if err != nil {
				fmt.Fprintf(os.Stderr, "  error: %v\n", err)
			} else {
				fmt.Fprintf(os.Stderr, "  latency: %s\n", lat)
				res.Latency = lat
			}
		} else {
			latencyCh := make(chan measure.Progress, 10)
			p := newProgressModel("ping", latencyCh)
			prog := tea.NewProgram(p)

			go func() {
				lat, err := b.TestLatency(ctx, serverList[0])
				if err != nil {
					latencyCh <- measure.Progress{Finished: true}
					return
				}
				latencyCh <- measure.Progress{Mbps: float64(lat.Microseconds()) / 1000.0, Finished: true}
			}()

			m, err := prog.Run()
			if err != nil {
				return fmt.Errorf("tui error: %w", err)
			}
			latMs := m.(progressModel).finalMbps
			res.Latency = time.Duration(latMs * float64(time.Millisecond))
		}
	}

	if runDownload {
		dlCh := make(chan measure.Progress, 10)
		verboseCh := make(chan measure.VerboseLog, 100)

		if verbose {
			fmt.Fprintln(os.Stderr, "\n--- Download ---")
			go func() {
				defer close(verboseCh)
				if err := b.TestDownload(ctx, serverList, opts, dlCh, verboseCh); err != nil {
					verboseCh <- measure.VerboseLog{Msg: fmt.Sprintf("error: %v", err)}
					dlCh <- measure.Progress{Finished: true}
				}
			}()
			for msg := range verboseCh {
				fmt.Fprintln(os.Stderr, "  "+msg.Msg)
			}
			for {
				p := <-dlCh
				if p.Finished {
					res.DownloadMbps = p.Mbps
					break
				}
			}
		} else {
			p := newProgressModel("download", dlCh)
			prog := tea.NewProgram(p)

			go func() {
				if err := b.TestDownload(ctx, serverList, opts, dlCh, verboseCh); err != nil {
					dlCh <- measure.Progress{Finished: true}
					return
				}
			}()

			m, err := prog.Run()
			if err != nil {
				return fmt.Errorf("tui error: %w", err)
			}
			res.DownloadMbps = m.(progressModel).finalMbps
		}
	}

	if runUpload {
		ulCh := make(chan measure.Progress, 10)
		verboseCh := make(chan measure.VerboseLog, 100)

		if verbose {
			fmt.Fprintln(os.Stderr, "\n--- Upload ---")
			go func() {
				defer close(verboseCh)
				if err := b.TestUpload(ctx, serverList, opts, ulCh, verboseCh); err != nil {
					verboseCh <- measure.VerboseLog{Msg: fmt.Sprintf("error: %v", err)}
					ulCh <- measure.Progress{Finished: true}
				}
			}()
			for msg := range verboseCh {
				fmt.Fprintln(os.Stderr, "  "+msg.Msg)
			}
			for {
				p := <-ulCh
				if p.Finished {
					res.UploadMbps = p.Mbps
					break
				}
			}
		} else {
			p := newProgressModel("upload", ulCh)
			prog := tea.NewProgram(p)

			go func() {
				if err := b.TestUpload(ctx, serverList, opts, ulCh, verboseCh); err != nil {
					ulCh <- measure.Progress{Finished: true}
					return
				}
			}()

			m, err := prog.Run()
			if err != nil {
				return fmt.Errorf("tui error: %w", err)
			}
			res.UploadMbps = m.(progressModel).finalMbps
		}
	}

	fmt.Println()
	if jsonOutput {
		jsonStr, err := res.JSON()
		if err != nil {
			return err
		}
		fmt.Println(jsonStr)
	} else {
		fmt.Println(res.String())
	}

	return nil
}

// bubbletea progress model

type progressModel struct {
	phase     string
	mbps      float64
	bytes     int64
	finished  bool
	finalMbps float64
	ch        <-chan measure.Progress
}

var (
	spinnerStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))
	phaseStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("241"))
	mbpsStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("86"))
	doneStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("46"))
)

func newProgressModel(phase string, ch <-chan measure.Progress) progressModel {
	return progressModel{
		phase: phase,
		ch:    ch,
	}
}

func (m progressModel) Init() tea.Cmd {
	return tickCmd()
}

func (m progressModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "q" || msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
	case tickMsg:
		for {
			select {
			case p := <-m.ch:
				m.mbps = p.Mbps
				m.bytes = p.Bytes
				if p.Finished {
					m.finished = true
					m.finalMbps = p.Mbps
					return m, tea.Quit
				}
			default:
				return m, tickCmd()
			}
		}
	}
	return m, nil
}

func (m progressModel) View() string {
	if m.finished {
		return doneStyle.Render(fmt.Sprintf("  %s ✓  %.2f Mbps", capitalize(m.phase), m.mbps))
	}

	spinner := spinnerStyle.Render("⟳")
	phase := phaseStyle.Render(fmt.Sprintf("  %s... ", capitalize(m.phase)))
	speed := mbpsStyle.Render(fmt.Sprintf("%.2f Mbps", m.mbps))

	return spinner + phase + speed
}

type tickMsg struct{}

func tickCmd() tea.Cmd {
	return tea.Tick(200*time.Millisecond, func(time.Time) tea.Msg {
		return tickMsg{}
	})
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
