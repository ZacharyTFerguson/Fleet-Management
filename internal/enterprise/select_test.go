package enterprise

import (
	"strings"
	"testing"
)

func TestSelectFilesBeatCDPAndPassword(t *testing.T) {
	ad, err := Select(FileAdapter{Fuel: "details.csv"}, SessionConfig{
		CDPURL:   "http://127.0.0.1:9222",
		Username: "u",
		Password: "p",
		CustNum:  "1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := ad.(FileAdapter); !ok {
		t.Fatalf("files must win, got %T", ad)
	}
}

func TestSelectCDPBeatsPasswordHTTP(t *testing.T) {
	ad, err := Select(FileAdapter{}, SessionConfig{
		CDPURL:     "http://127.0.0.1:9222",
		Username:   "u",
		Password:   "p",
		CustNum:    "1",
		DetailsURL: "https://example.invalid/DETAILS.csv",
	})
	if err != nil {
		t.Fatal(err)
	}
	chrome, ok := ad.(*ChromeSessionAdapter)
	if !ok {
		t.Fatalf("CDP must win over password HTTP, got %T", ad)
	}
	if chrome.DetailsURL == "" {
		t.Fatal("captured DETAILS URL must ride with the CDP adapter")
	}
}

func TestSelectPasswordHTTPIsLastResort(t *testing.T) {
	ad, err := Select(FileAdapter{}, SessionConfig{
		Username: "u",
		Password: "p",
		CustNum:  "1",
		BaseURL:  "https://login.example.invalid",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := ad.(*HTTPAdapter); !ok {
		t.Fatalf("password HTTP last, got %T", ad)
	}
}

func TestSelectNeedsAPath(t *testing.T) {
	_, err := Select(FileAdapter{}, SessionConfig{})
	if err == nil || !strings.Contains(err.Error(), "EFLEETS_CDP_URL") {
		t.Fatalf("want CDP-or-files hint, got %v", err)
	}
}
