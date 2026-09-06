package app

import (
	"context"
	"fmt"
	"strings"

	"oilchange/internal/deskauth"
	"oilchange/internal/onestep"
	"oilchange/internal/places"
	"oilchange/internal/store"
)

const sendConfirmPhrase = "SEND_TO_ONESTEP"

type MarkerList struct {
	Jobs        []store.MarkerJob `json:"jobs"`
	Places      int               `json:"places"`
	Note        string            `json:"note"`
	WriteProven bool              `json:"write_proven"`
	PortalURL   string            `json:"portal_url"`
}

func (a *App) ListMarkerJobs(ctx context.Context) (MarkerList, error) {
	jobs, err := a.Store.ListMarkerJobs(ctx)
	if err != nil {
		return MarkerList{}, err
	}
	pls, err := a.Store.ListPlaces(ctx)
	if err != nil {
		return MarkerList{}, err
	}
	note := "Gas Stations only (type 001 / Gas_Stations). Download uses proven GET /zone-group + /zone. API create is not proven — portal-first until ONESTEP_WRITE_PROVEN=1. Miles still come from drive-stop, never from this page."
	return MarkerList{
		Jobs:        jobs,
		Places:      len(pls),
		Note:        note,
		WriteProven: onestep.WriteProven(),
		PortalURL:   onestep.PortalMapURL,
	}, nil
}

// PullOneStepGasStations downloads live Gas_Stations zones (proven list APIs) into the catalog.
func (a *App) PullOneStepGasStations(ctx context.Context) (MarkerList, error) {
	c := a.oneStepClient()
	if c == nil || c.Token == "" {
		return MarkerList{}, fmt.Errorf("OneStep API key missing")
	}
	items, _, err := c.ListGasStationZones(ctx)
	if err != nil {
		return MarkerList{}, err
	}
	for _, it := range items {
		if err := a.upsertOneStepGasPlace(ctx, it); err != nil {
			return MarkerList{}, err
		}
	}
	return a.ListMarkerJobs(ctx)
}

func (a *App) upsertOneStepGasPlace(ctx context.Context, it onestep.PlaceItem) error {
	general, typeCode, brand, top, grade, ok := places.ParseCanonLabel(it.Name)
	if !ok {
		return nil
	}
	if err := places.GasOnly(typeCode); err != nil {
		return err
	}
	p, err := a.Store.GetPlace(ctx, general)
	if err != nil {
		return err
	}
	if p == nil {
		created, err := places.NewGasPlace(general, it.Name, it.Address, "", "onestep_zone")
		if err != nil {
			return err
		}
		created.BrandCode = brand
		created.TopTier = top
		created.TopTierGrade = grade
		created.Label = places.LabelOf(general, typeCode, brand, top, grade)
		created.Name = it.Name
		p = &created
	}
	p.OneStepZoneID = it.ID
	if it.Address != "" {
		p.Address = it.Address
	}
	if it.Lat != nil {
		p.Lat = it.Lat
	}
	if it.Lng != nil {
		p.Lng = it.Lng
	}
	if err := a.Store.UpsertPlace(ctx, *p); err != nil {
		return err
	}
	draft := places.DraftFor(*p)
	if it.Lat != nil {
		draft.Lat = it.Lat
	}
	if it.Lng != nil {
		draft.Lng = it.Lng
	}
	id := "job-" + p.GeneralCode
	existing, _, _ := a.Store.GetMarkerJob(ctx, id)
	stage := "onestep"
	if existing != nil && existing.Stage != "" && existing.Stage != "review" {
		stage = existing.Stage
	}
	return a.Store.UpsertMarkerJob(ctx, id, p.GeneralCode, stage, onestep.CompactJSON(draft), "")
}

// PullGasCandidates seeds places + review jobs from gas_stations and fuel merchants.
// Shop / maintenance names are skipped.
func (a *App) PullGasCandidates(ctx context.Context) (MarkerList, error) {
	stations, err := a.Store.ListStations(ctx)
	if err != nil {
		return MarkerList{}, err
	}
	fills, err := a.Store.ListAllFills(ctx)
	if err != nil {
		return MarkerList{}, err
	}
	existing, err := a.Store.ListPlaces(ctx)
	if err != nil {
		return MarkerList{}, err
	}
	seen := map[string]string{}
	last := ""
	for _, p := range existing {
		seen[places.StationKey(p.Name, p.Address)] = p.GeneralCode
		if p.GeneralCode > last {
			last = p.GeneralCode
		}
	}

	add := func(name, address, merchant, source string) error {
		if !places.IsGasMerchant(name, address) {
			return nil
		}
		key := places.StationKey(name, address)
		if key == "|" {
			return nil
		}
		if _, ok := seen[key]; ok {
			return nil
		}
		next, err := places.NextGeneralCode(last)
		if err != nil {
			return err
		}
		p, err := places.NewGasPlace(next, name, address, merchant, source)
		if err != nil {
			return err
		}
		if err := a.Store.UpsertPlace(ctx, p); err != nil {
			return err
		}
		draft := places.DraftFor(p)
		id := "job-" + p.GeneralCode
		if err := a.Store.UpsertMarkerJob(ctx, id, p.GeneralCode, "review", onestep.CompactJSON(draft), ""); err != nil {
			return err
		}
		seen[key] = p.GeneralCode
		last = p.GeneralCode
		return nil
	}

	for _, g := range stations {
		if err := add(g.Name, g.Address, g.MerchantID, "gas_stations"); err != nil {
			return MarkerList{}, err
		}
	}
	for _, f := range fills {
		if err := add(f.MerchantName, f.MerchantAddress, "", "fills"); err != nil {
			return MarkerList{}, err
		}
	}
	return a.ListMarkerJobs(ctx)
}

func (a *App) GeocodeMarker(ctx context.Context, id string) (*store.MarkerJob, error) {
	j, confirm, err := a.Store.GetMarkerJob(ctx, id)
	if err != nil || j == nil {
		if err == nil {
			err = fmt.Errorf("job not found")
		}
		return nil, err
	}
	addr := j.Draft.Address
	if addr == "" {
		return nil, fmt.Errorf("no address to geocode")
	}
	provider := a.SecretPlain(ctx, "geocoder_provider")
	key := a.SecretPlain(ctx, "geocoder_api_key")
	res, err := places.Geocode(ctx, provider, key, addr)
	if err != nil {
		j.LastError = err.Error()
		_ = a.Store.UpdateMarkerJob(ctx, *j, onestep.CompactJSON(j.Draft), confirm)
		return nil, err
	}
	j.ThirdPartyLat = &res.Lat
	j.ThirdPartyLng = &res.Lng
	j.ThirdPartyProvider = res.Provider
	j.Draft.Lat = &res.Lat
	j.Draft.Lng = &res.Lng
	j.Stage = "geocoded"
	j.LastError = ""
	j.MapURL = res.MapURL
	if err := a.Store.UpdateMarkerJob(ctx, *j, onestep.CompactJSON(j.Draft), confirm); err != nil {
		return nil, err
	}
	p, err := a.Store.GetPlace(ctx, j.GeneralCode)
	if err == nil && p != nil {
		p.Lat = &res.Lat
		p.Lng = &res.Lng
		_ = a.Store.UpsertPlace(ctx, *p)
	}
	return j, nil
}

func (a *App) ReviewMarker(ctx context.Context, id, action, notes string) (*store.MarkerJob, error) {
	j, confirm, err := a.Store.GetMarkerJob(ctx, id)
	if err != nil || j == nil {
		if err == nil {
			err = fmt.Errorf("job not found")
		}
		return nil, err
	}
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "approve":
		j.Stage = "approved"
		j.ReviewNotes = notes
		j.LastError = ""
		confirm = deskauth.RandomConfirm()
	case "hold":
		j.Stage = "hold"
		j.ReviewNotes = notes
		confirm = ""
	default:
		return nil, fmt.Errorf("action must be approve or hold")
	}
	if err := a.Store.UpdateMarkerJob(ctx, *j, onestep.CompactJSON(j.Draft), confirm); err != nil {
		return nil, err
	}
	j.HasConfirm = confirm != ""
	return j, nil
}

func (a *App) DryRunMarker(ctx context.Context, id string) (map[string]any, error) {
	j, _, err := a.Store.GetMarkerJob(ctx, id)
	if err != nil || j == nil {
		if err == nil {
			err = fmt.Errorf("job not found")
		}
		return nil, err
	}
	payload, err := markerPayload(j)
	if err != nil {
		return nil, err
	}
	c := a.oneStepClient()
	if c == nil || c.Token == "" {
		return map[string]any{
			"dry_run":    true,
			"would_send": payload,
			"note":       "OneStep API key missing — dry-run local only. No POST.",
		}, nil
	}
	out, err := c.DryRunMarker(ctx, payload)
	if err != nil {
		return nil, err
	}
	j.Stage = "dry_run"
	_ = a.Store.UpdateMarkerJob(ctx, *j, onestep.CompactJSON(j.Draft), "")
	return out, nil
}

func (a *App) SendMarker(ctx context.Context, id, confirmPhrase, confirmToken string) (*store.MarkerJob, error) {
	if confirmPhrase != sendConfirmPhrase {
		return nil, fmt.Errorf("send refused: type %s to confirm (no bulk)", sendConfirmPhrase)
	}
	j, stored, err := a.Store.GetMarkerJob(ctx, id)
	if err != nil || j == nil {
		if err == nil {
			err = fmt.Errorf("job not found")
		}
		return nil, err
	}
	if j.Stage != "approved" && j.Stage != "dry_run" && j.Stage != "geocoded" && j.Stage != "sent" {
		return nil, fmt.Errorf("review/approve this gas station before send (stage %s)", j.Stage)
	}
	if stored == "" || confirmToken == "" || stored != confirmToken {
		return nil, fmt.Errorf("confirm token mismatch — approve again to get a fresh token")
	}
	payload, err := markerPayload(j)
	if err != nil {
		return nil, err
	}
	c := a.oneStepClient()
	if c == nil || c.Token == "" {
		return nil, fmt.Errorf("OneStep API key missing")
	}
	mid, path, _, err := c.CreateMarker(ctx, payload)
	if err != nil {
		j.LastError = err.Error()
		_ = a.Store.UpdateMarkerJob(ctx, *j, onestep.CompactJSON(j.Draft), stored)
		return nil, err
	}
	j.Stage = "sent"
	j.LastError = ""
	if err := a.Store.UpdateMarkerJob(ctx, *j, onestep.CompactJSON(j.Draft), ""); err != nil {
		return nil, err
	}
	p, err := a.Store.GetPlace(ctx, j.GeneralCode)
	if err == nil && p != nil {
		p.OneStepMarkerID = mid
		if p.OneStepMarkerID == "" {
			p.OneStepMarkerID = path
		}
		_ = a.Store.UpsertPlace(ctx, *p)
	}
	_ = a.recompareOneStep(ctx, j)
	return j, nil
}

func (a *App) SendZone(ctx context.Context, id, confirmPhrase, confirmToken string) (*store.MarkerJob, error) {
	if confirmPhrase != sendConfirmPhrase {
		return nil, fmt.Errorf("zone send refused: type %s to confirm", sendConfirmPhrase)
	}
	j, stored, err := a.Store.GetMarkerJob(ctx, id)
	if err != nil || j == nil {
		if err == nil {
			err = fmt.Errorf("job not found")
		}
		return nil, err
	}
	if j.Stage != "sent" && j.Stage != "zone" {
		return nil, fmt.Errorf("create the marker first, then place a zone (stage %s)", j.Stage)
	}
	if stored == "" || stored != confirmToken {
		// Allow a just-approved token after re-approve, or require re-approve after send.
		if confirmToken == "" {
			return nil, fmt.Errorf("approve again after marker send to unlock zone confirm")
		}
	}
	lat, lng, err := jobLatLng(j)
	if err != nil {
		return nil, err
	}
	c := a.oneStepClient()
	if c == nil || c.Token == "" {
		return nil, fmt.Errorf("OneStep API key missing")
	}
	zid, _, err := c.CreateZoneNear(ctx, j.Draft.Name, lat, lng, places.ZoneRadiusM)
	if err != nil {
		j.LastError = err.Error()
		_ = a.Store.UpdateMarkerJob(ctx, *j, onestep.CompactJSON(j.Draft), stored)
		return nil, err
	}
	j.Stage = "zone"
	j.LastError = ""
	if err := a.Store.UpdateMarkerJob(ctx, *j, onestep.CompactJSON(j.Draft), ""); err != nil {
		return nil, err
	}
	p, err := a.Store.GetPlace(ctx, j.GeneralCode)
	if err == nil && p != nil {
		p.OneStepZoneID = zid
		_ = a.Store.UpsertPlace(ctx, *p)
	}
	_ = a.recompareOneStep(ctx, j)
	return j, nil
}

func (a *App) recompareOneStep(ctx context.Context, j *store.MarkerJob) error {
	c := a.oneStepClient()
	if c == nil || c.Token == "" {
		return nil
	}
	items, _, _, err := c.ListPlaces(ctx)
	if err != nil {
		return err
	}
	hit := onestep.FindByName(items, j.Draft.Name)
	if hit == nil || hit.Lat == nil || hit.Lng == nil {
		return nil
	}
	j.OneStepLat = hit.Lat
	j.OneStepLng = hit.Lng
	_, confirm, _ := a.Store.GetMarkerJob(ctx, j.ID)
	return a.Store.UpdateMarkerJob(ctx, *j, onestep.CompactJSON(j.Draft), confirm)
}

func markerPayload(j *store.MarkerJob) (onestep.MarkerPayload, error) {
	lat, lng, err := jobLatLng(j)
	if err != nil {
		return onestep.MarkerPayload{}, err
	}
	p := onestep.MarkerPayload{
		Name:    j.Draft.Name,
		Address: j.Draft.Address,
		Lat:     lat,
		Lng:     lng,
		Group:   places.GroupGas,
		Type:    "gas",
		RadiusM: places.ZoneRadiusM,
		Color:   "#f0c040",
	}
	return p, p.ValidateGas()
}

func jobLatLng(j *store.MarkerJob) (float64, float64, error) {
	if j.ThirdPartyLat != nil && j.ThirdPartyLng != nil {
		return *j.ThirdPartyLat, *j.ThirdPartyLng, nil
	}
	if j.Draft.Lat != nil && j.Draft.Lng != nil {
		return *j.Draft.Lat, *j.Draft.Lng, nil
	}
	return 0, 0, fmt.Errorf("geocode this station before send")
}

func (a *App) Login(ctx context.Context, username, password string) (string, bool, error) {
	if err := deskauth.ValidateUsername(username); err != nil {
		return "", false, err
	}
	if strings.TrimSpace(password) == "" {
		return "", false, fmt.Errorf("password required")
	}
	envUser, envPass := deskauth.EnvDeskUser()
	if envUser != "" && envPass != "" && username == envUser && password == envPass {
		return username, false, nil
	}
	n, err := a.Store.CountDeskUsers(ctx)
	if err != nil {
		return "", false, err
	}
	if n == 0 {
		hash, err := deskauth.HashPassword(password)
		if err != nil {
			return "", false, err
		}
		if err := a.Store.CreateDeskUser(ctx, username, hash); err != nil {
			return "", false, err
		}
		_ = a.Store.TouchDeskLogin(ctx, username)
		return username, true, nil
	}
	u, err := a.Store.GetDeskUser(ctx, username)
	if err != nil || u == nil {
		return "", false, fmt.Errorf("invalid login")
	}
	if !deskauth.CheckPassword(u.PasswordHash, password) {
		return "", false, fmt.Errorf("invalid login")
	}
	_ = a.Store.TouchDeskLogin(ctx, username)
	return username, false, nil
}

func (a *App) SessionMeta(ctx context.Context) (users int, bootstrap bool, err error) {
	n, err := a.Store.CountDeskUsers(ctx)
	if err != nil {
		return 0, false, err
	}
	envUser, envPass := deskauth.EnvDeskUser()
	return n, n == 0 && (envUser == "" || envPass == ""), nil
}

// ConfirmTokenFor is used by the UI after approve. It is a send gate, not a credential.
func (a *App) ConfirmTokenFor(ctx context.Context, id string) (string, error) {
	_, tok, err := a.Store.GetMarkerJob(ctx, id)
	return tok, err
}
