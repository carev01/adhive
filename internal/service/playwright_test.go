package service

import (
	"testing"

	"github.com/carev01/adhive/internal/model"
)

func TestShouldUseManualMode(t *testing.T) {
	tests := []struct {
		domains      []string
		enableManual bool
		url          string
		wantManual   bool
	}{
		{[]string{"private55.com"}, true, "https://private55.com/page", true},
		{[]string{"private55.com"}, false, "https://private55.com/page", false},
		{[]string{"example.com"}, true, "https://private55.com/page", false},
		{[]string{"private55.com"}, true, "https://www.private55.com/page", true},
		{[]string{"private55.com"}, true, "https://other.com/page", false},
		{[]string{}, true, "https://anything.com", false},
	}

	for _, tt := range tests {
		cfg := DefaultPlaywrightConfig()
		cfg.HumanDomains = tt.domains
		cfg.EnableManualCapture = tt.enableManual
		s := NewPlaywrightService(cfg)
		got := s.shouldUseManualMode(tt.url)
		if got != tt.wantManual {
			t.Errorf("shouldUseManualMode(domains=%v,enable=%v,url=%s) = %v; want %v",
				tt.domains, tt.enableManual, tt.url, got, tt.wantManual)
		}
	}
}

func TestManifestStatus_ChallengeDetected(t *testing.T) {
	capture := &PlaywrightResult{
		HTML:              "<html>challenge page</html>",
		StatusCode:        200,
		ChallengeDetected: true,
		ChallengeSignals:  []string{"captcha", "cloudflare"},
	}
	stats := model.ArchiveManifestStats{
		TotalAssets:      0,
		DownloadedAssets: 0,
		TotalBytes:       0,
	}
	status := manifestStatus(capture, stats)
	if status != model.ArchiveRevisionStatusBlocked {
		t.Errorf("manifestStatus with challenge = %v; want blocked", status)
	}
}

func TestManifestStatus_Partial_WithErrorAssets(t *testing.T) {
	capture := &PlaywrightResult{
		HTML:              "<html>content</html>",
		StatusCode:        200,
		ChallengeDetected: false,
	}
	stats := model.ArchiveManifestStats{
		TotalAssets:      10,
		DownloadedAssets: 8,
		ErrorAssets:      2,
		TotalBytes:       1024,
	}
	status := manifestStatus(capture, stats)
	if status != model.ArchiveRevisionStatusPartial {
		t.Errorf("manifestStatus with error assets = %v; want partial", status)
	}
}

func TestManifestStatus_Partial_WithHttpErrorAndSomeAssets(t *testing.T) {
	capture := &PlaywrightResult{
		HTML:              "",
		StatusCode:        500,
		Error:             "server error",
		ChallengeDetected: false,
	}
	stats := model.ArchiveManifestStats{
		TotalAssets:      5,
		DownloadedAssets: 2,
		TotalBytes:       512,
	}
	status := manifestStatus(capture, stats)
	if status != model.ArchiveRevisionStatusPartial {
		t.Errorf("manifestStatus with HTTP error and some assets = %v; want partial", status)
	}
}

func TestManifestStatus_Failed_NoAssetsOnError(t *testing.T) {
	capture := &PlaywrightResult{
		HTML:              "",
		StatusCode:        500,
		Error:             "server error",
		ChallengeDetected: false,
	}
	stats := model.ArchiveManifestStats{
		TotalAssets:      0,
		DownloadedAssets: 0,
		TotalBytes:       0,
	}
	status := manifestStatus(capture, stats)
	if status != model.ArchiveRevisionStatusFailed {
		t.Errorf("manifestStatus with HTTP error and no assets = %v; want failed", status)
	}
}

func TestManifestStatus_Success(t *testing.T) {
	capture := &PlaywrightResult{
		HTML:              "<html>full content</html>",
		StatusCode:        200,
		ChallengeDetected: false,
	}
	stats := model.ArchiveManifestStats{
		TotalAssets:      5,
		DownloadedAssets: 5,
		TotalBytes:       2048,
	}
	status := manifestStatus(capture, stats)
	if status != model.ArchiveRevisionStatusSuccess {
		t.Errorf("manifestStatus full success = %v; want success", status)
	}
}
