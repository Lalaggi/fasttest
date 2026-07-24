package fastcom

import (
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
)

const fallbackToken = "YXNkZmFzZGxmbnNkYWZoYXNkZmhrYWxm"

var tokenRe = regexp.MustCompile(`token:"([^"]+)"`)
var scriptRe = regexp.MustCompile(`src="(app-[^"]+\.js)"`)

func getToken() (string, error) {
	if valid, err := validateToken(fallbackToken); err == nil && valid {
		return fallbackToken, nil
	}

	token, err := scrapeToken()
	if err != nil {
		return "", fmt.Errorf("failed to obtain token: %w", err)
	}
	return token, nil
}

func validateToken(token string) (bool, error) {
	url := fmt.Sprintf("https://api.fast.com/netflix/speedtest/v2?https=true&token=%s&urlCount=1", token)
	resp, err := http.Get(url)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK, nil
}

func scrapeToken() (string, error) {
	resp, err := http.Get("https://fast.com/")
	if err != nil {
		return "", fmt.Errorf("fetching fast.com: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading fast.com body: %w", err)
	}

	html := string(body)
	scriptMatch := scriptRe.FindStringSubmatch(html)
	if len(scriptMatch) < 2 {
		return "", fmt.Errorf("no app script found in fast.com HTML")
	}

	scriptURL := "https://fast.com/" + scriptMatch[1]
	resp2, err := http.Get(scriptURL)
	if err != nil {
		return "", fmt.Errorf("fetching app script: %w", err)
	}
	defer resp2.Body.Close()

	body2, err := io.ReadAll(resp2.Body)
	if err != nil {
		return "", fmt.Errorf("reading app script body: %w", err)
	}

	tokenMatch := tokenRe.FindStringSubmatch(string(body2))
	if len(tokenMatch) < 2 {
		return "", fmt.Errorf("no token found in app script")
	}

	token := strings.TrimSpace(tokenMatch[1])
	if token == "" {
		return "", fmt.Errorf("empty token found in app script")
	}

	return token, nil
}
