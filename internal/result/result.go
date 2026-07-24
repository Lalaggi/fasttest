package result

import (
	"encoding/json"
	"fmt"
	"math"
	"time"

	"github.com/charmbracelet/lipgloss"
)

type Result struct {
	DownloadMbps float64       `json:"download_mbps"`
	UploadMbps   float64       `json:"upload_mbps"`
	Latency      time.Duration `json:"latency"`
	ServerIP     string        `json:"server_ip"`
	Location     string        `json:"location"`
	ASN          string        `json:"asn"`
}

var (
	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("212"))
	valueStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("255"))
	unitStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))
)

func roundTo(x float64, n int) float64 {
	if x == 0 {
		return 0
	}
	pow := math.Pow(10, float64(n-1))
	shifted := x * pow
	rounded := math.Round(shifted)
	return rounded / pow
}

func formatLatency(d time.Duration) string {
	ms := float64(d.Microseconds()) / 1000.0
	return fmt.Sprintf("%.3g ms", roundTo(ms, 3))
}

func (r Result) String() string {
	var out string

	out += headerStyle.Render("Download") + "\t" + valueStyle.Render(fmt.Sprintf("%.2f", r.DownloadMbps)) + unitStyle.Render(" Mbps") + "\n"
	out += headerStyle.Render("Upload") + "\t\t" + valueStyle.Render(fmt.Sprintf("%.2f", r.UploadMbps)) + unitStyle.Render(" Mbps") + "\n"
	out += headerStyle.Render("Latency") + "\t" + valueStyle.Render(formatLatency(r.Latency)) + "\n"

	if r.Location != "" {
		out += headerStyle.Render("Server") + "\t" + valueStyle.Render(r.Location) + "\n"
	}
	if r.ASN != "" {
		out += headerStyle.Render("ASN") + "\t\t" + valueStyle.Render(r.ASN) + "\n"
	}

	return out
}

func (r Result) JSON() (string, error) {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshaling result: %w", err)
	}
	return string(data), nil
}
