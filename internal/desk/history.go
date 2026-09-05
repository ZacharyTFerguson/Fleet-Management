package desk

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"oilchange/internal/history"
	"oilchange/internal/model"
	"oilchange/internal/onestep"
)

// HistoryAPI is the sqlite-backed History / Devices GPS surface.
type HistoryAPI struct {
	Board    func(ctx context.Context, region string) (history.Board, error)
	Assign   func(ctx context.Context, txKey, toEFleets, reason, region string) (history.Board, error)
	Events   func(ctx context.Context, txKey string) ([]model.AssignmentEvent, error)
	Evidence func(ctx context.Context, txKey string) (history.FillEvidence, error)
	Box      func(ctx context.Context, factoryID, deviceID string) (history.BoxEvidence, error)
	Probe    func(ctx context.Context, factoryID, deviceID, txKey string, hours int) (onestep.ProbeResult, error)
}

type assignBody struct {
	TxKey     string `json:"tx_key"`
	ToEFleets string `json:"to_efleets_id"`
	Reason    string `json:"reason"`
	Region    string `json:"region"`
}

type probeBody struct {
	FactoryID string `json:"factory_id"`
	DeviceID  string `json:"device_id"`
	TxKey     string `json:"tx_key"`
	Hours     int    `json:"hours"`
}

func serveHistory(w http.ResponseWriter, r *http.Request, api *HistoryAPI) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if api == nil || api.Board == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "oilchange serve has no sqlite store — set OILCHANGE_DB for History"})
		return
	}
	switch r.Method {
	case http.MethodGet, http.MethodHead:
		board, err := api.Board(r.Context(), r.URL.Query().Get("region"))
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusOK)
			return
		}
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		_ = enc.Encode(board)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func serveHistoryAssign(w http.ResponseWriter, r *http.Request, api *HistoryAPI) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if api == nil || api.Assign == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "oilchange serve has no sqlite store — set OILCHANGE_DB for History"})
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body assignBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid json"})
		return
	}
	board, err := api.Assign(r.Context(), body.TxKey, body.ToEFleets, body.Reason, body.Region)
	if err != nil {
		code := http.StatusBadRequest
		if strings.Contains(err.Error(), "no store") {
			code = http.StatusServiceUnavailable
		}
		w.WriteHeader(code)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(board)
}

func serveHistoryEvents(w http.ResponseWriter, r *http.Request, api *HistoryAPI) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if api == nil || api.Events == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "oilchange serve has no sqlite store — set OILCHANGE_DB for History"})
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	ev, err := api.Events(r.Context(), r.URL.Query().Get("tx_key"))
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	if ev == nil {
		ev = []model.AssignmentEvent{}
	}
	if r.Method == http.MethodHead {
		return
	}
	_ = json.NewEncoder(w).Encode(ev)
}

func serveDeviceEvidence(w http.ResponseWriter, r *http.Request, api *HistoryAPI) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if api == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "oilchange serve has no sqlite store — set OILCHANGE_DB for Devices"})
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	q := r.URL.Query()
	txKey := strings.TrimSpace(q.Get("tx_key"))
	factory := strings.TrimSpace(q.Get("factory_id"))
	device := strings.TrimSpace(q.Get("device_id"))
	if txKey != "" {
		if api.Evidence == nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "evidence API off"})
			return
		}
		ev, err := api.Evidence(r.Context(), txKey)
		if err != nil {
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		if r.Method == http.MethodHead {
			return
		}
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		_ = enc.Encode(ev)
		return
	}
	if factory != "" || device != "" {
		if api.Box == nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "box API off"})
			return
		}
		box, err := api.Box(r.Context(), factory, device)
		if err != nil {
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		if r.Method == http.MethodHead {
			return
		}
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		_ = enc.Encode(box)
		return
	}
	w.WriteHeader(http.StatusBadRequest)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": "tx_key or factory_id or device_id required"})
}

func serveDeviceProbe(w http.ResponseWriter, r *http.Request, api *HistoryAPI) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if api == nil || api.Probe == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "oilchange serve has no sqlite store — set OILCHANGE_DB for Devices"})
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body probeBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid json"})
		return
	}
	if body.Hours == 0 {
		if h, _ := strconv.Atoi(r.URL.Query().Get("hours")); h > 0 {
			body.Hours = h
		}
	}
	res, err := api.Probe(r.Context(), body.FactoryID, body.DeviceID, body.TxKey, body.Hours)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(map[string]any{
		"device_id": res.DeviceID,
		"from":      res.From,
		"to":        res.To,
		"miles":     res.Miles,
		"auth":      res.AuthMode,
		"elapsed":   res.Elapsed.String(),
	})
}
