package onestep

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// StatusError is a non-2xx OneStep HTTP response. RetryAfter is set when the
// API sends Retry-After (429 / 503). Callers must wait; get() does not sleep.
type StatusError struct {
	Path       string
	Status     string
	StatusCode int
	RetryAfter time.Duration
	Msg        string
}

func (e *StatusError) Error() string {
	if e == nil {
		return "onestep: HTTP error"
	}
	if strings.TrimSpace(e.Msg) == "" {
		return fmt.Sprintf("onestep %s: HTTP %s", e.Path, e.Status)
	}
	return fmt.Sprintf("onestep %s: HTTP %s: %s", e.Path, e.Status, e.Msg)
}

// RetryAfterOf returns the API-requested wait, or 0.
func RetryAfterOf(err error) time.Duration {
	var se *StatusError
	if errors.As(err, &se) && se != nil {
		return se.RetryAfter
	}
	return 0
}

// ParseRetryAfter accepts delta-seconds or an HTTP-date. Invalid / past → 0.
func ParseRetryAfter(v string) time.Duration {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0
	}
	if n, err := strconv.Atoi(v); err == nil {
		if n <= 0 {
			return 0
		}
		return time.Duration(n) * time.Second
	}
	t, err := http.ParseTime(v)
	if err != nil {
		return 0
	}
	d := time.Until(t)
	if d <= 0 {
		return 0
	}
	return d
}

func httpStatusError(path string, res *http.Response, body []byte, maxBody int, secrets ...string) error {
	msg := strings.TrimSpace(string(body))
	msg = sanitizeAuthError(msg, secrets...)
	if maxBody <= 0 {
		maxBody = 240
	}
	if len(msg) > maxBody {
		msg = msg[:maxBody] + "…"
	}
	status := ""
	code := 0
	var retry time.Duration
	if res != nil {
		status = res.Status
		code = res.StatusCode
		retry = ParseRetryAfter(res.Header.Get("Retry-After"))
	}
	return &StatusError{Path: path, Status: status, StatusCode: code, RetryAfter: retry, Msg: msg}
}
