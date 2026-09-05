package desk

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// AppWindowCmd returns a Chrome/Edge --app= command so the desk opens as a
// window without browser chrome. Empty look uses PATH lookups.
func AppWindowCmd(url string, look []string) *exec.Cmd {
	url = strings.TrimSpace(url)
	if url == "" {
		return nil
	}
	if len(look) == 0 {
		look = defaultAppBrowsers()
	}
	for _, bin := range look {
		path, err := exec.LookPath(bin)
		if err != nil {
			continue
		}
		return exec.Command(path, "--app="+url)
	}
	return nil
}

func defaultAppBrowsers() []string {
	switch runtime.GOOS {
	case "windows":
		return []string{"msedge", "chrome", "chromium"}
	case "darwin":
		return []string{"google-chrome", "chromium", "microsoft-edge"}
	default:
		return []string{"google-chrome", "google-chrome-stable", "chromium", "chromium-browser", "microsoft-edge", "msedge"}
	}
}

// OpenAppWindow starts a standalone browser window. It does not start the server.
func OpenAppWindow(url string) error {
	if cmd := AppWindowCmd(url, nil); cmd != nil {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Start()
	}
	return fmt.Errorf("no Chrome/Edge on PATH for --app window; oilchange serve still works in a browser")
}

// WaitHTTP polls until GET url succeeds or timeout.
func WaitHTTP(url string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 300 * time.Millisecond}
	for time.Now().Before(deadline) {
		res, err := client.Get(url)
		if err == nil {
			_ = res.Body.Close()
			if res.StatusCode < 500 {
				return nil
			}
		}
		time.Sleep(80 * time.Millisecond)
	}
	return fmt.Errorf("desk did not become ready at %s", url)
}
