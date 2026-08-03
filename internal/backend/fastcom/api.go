package fastcom

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/Lalaggi/fasttest/internal/util"
)

type target struct {
	URL      string `json:"url"`
	Location struct {
		City    string `json:"city"`
		Country string `json:"country"`
	} `json:"location"`
}

type clientInfo struct {
	IP   string `json:"ip"`
	ASN  string `json:"asn"`
	Location struct {
		City    string `json:"city"`
		Country string `json:"country"`
	} `json:"location"`
}

type targetsResponse struct {
	Client  clientInfo `json:"client"`
	Targets []target   `json:"targets"`
}

func getTargets(token string, count int) (clientInfo, []target, error) {
	url := fmt.Sprintf("https://api.fast.com/netflix/speedtest/v2?https=true&token=%s&urlCount=%d", token, count)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return clientInfo{}, nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Referer", "https://fast.com/")
	req.Header.Set("Origin", "https://fast.com")

	resp, err := util.HTTPClient.Do(req)
	if err != nil {
		return clientInfo{}, nil, fmt.Errorf("fetching targets: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return clientInfo{}, nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var result targetsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return clientInfo{}, nil, fmt.Errorf("decoding response: %w", err)
	}

	if len(result.Targets) == 0 {
		return clientInfo{}, nil, fmt.Errorf("no targets returned")
	}

	return result.Client, result.Targets, nil
}
