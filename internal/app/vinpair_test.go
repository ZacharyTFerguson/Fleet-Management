package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"oilchange/internal/config"
	"oilchange/internal/model"
	"oilchange/internal/onestep"
	"oilchange/internal/store"
)

func TestPairDevicesByVINLinksUnpairedFromLiveList(t *testing.T) {
	p := filepath.Join(t.TempDir(), "vin.sqlite")
	st, err := store.Open("sqlite", p)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	if err := st.UpsertCar(ctx, model.Car{EFleetsID: "27TESTA", VIN: "1HGCM82633A004352", Nickname: "WrongCar"}); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertDevice(ctx, model.OneStepDevice{FactoryID: "FACTVIN", DeviceID: "DEVVIN", DisplayName: "WrongCar", Active: true}); err != nil {
		t.Fatal(err)
	}
	a := &App{Store: st}
	live := []model.OneStepDevice{{
		FactoryID: "FACTVIN", DeviceID: "DEVVIN", DisplayName: "WrongCar",
		VIN: "1HGCM82633A004352", Active: true,
	}}
	res, err := a.PairDevicesByVIN(ctx, PairVINOpts{Live: live, AskEmpty: false})
	if err != nil {
		t.Fatal(err)
	}
	if res.Linked != 1 || len(res.Links) != 1 || res.Links[0].EFleetsID != "27TESTA" {
		t.Fatalf("%+v", res)
	}
	devs, err := st.ListDevicesForCar(ctx, "27TESTA")
	if err != nil {
		t.Fatal(err)
	}
	if len(devs) != 1 || devs[0].FactoryID != "FACTVIN" {
		t.Fatalf("%+v", devs)
	}
}

func TestPairDevicesByVINAsksEmptyVINAndDoesNotStealMap(t *testing.T) {
	p := filepath.Join(t.TempDir(), "ask.sqlite")
	st, err := store.Open("sqlite", p)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	keep := "MAPPED"
	if err := st.UpsertCar(ctx, model.Car{EFleetsID: keep, VIN: "1HGCM82633A000001"}); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertCar(ctx, model.Car{EFleetsID: "VINTARGET", VIN: "1HGCM82633A004352"}); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertDevice(ctx, model.OneStepDevice{
		FactoryID: "KEEP", DeviceID: "OLD", LinkedCarEFleetsID: &keep, Active: true,
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertDevice(ctx, model.OneStepDevice{
		FactoryID: "LOOSE", DeviceID: "DEVLOOSE", DisplayName: "WrongCar", Active: true,
	}); err != nil {
		t.Fatal(err)
	}
	var asked int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/device") {
			http.NotFound(w, r)
			return
		}
		atomic.AddInt32(&asked, 1)
		did := r.URL.Query().Get("device_id")
		if did == "DEVLOOSE" {
			_, _ = w.Write([]byte(`[{
				"factory_id":"LOOSE","device_id":"DEVLOOSE","display_name":"WrongCar",
				"latest_device_point":{"device_state":{"vin":"1HGCM82633A004352"}}
			}]`))
			return
		}
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()
	c := onestep.NewClient(srv.URL, "tok")
	c.HTTP = srv.Client()
	a := &App{Cfg: config.Config{OneStepToken: "tok"}, Store: st, OneStep: c}
	res, err := a.PairDevicesByVIN(ctx, PairVINOpts{AskEmpty: true, Pace: 1})
	if err != nil {
		t.Fatal(err)
	}
	if res.Linked != 1 {
		t.Fatalf("linked %+v", res)
	}
	loose, err := st.ListDevicesForCar(ctx, "VINTARGET")
	if err != nil {
		t.Fatal(err)
	}
	if len(loose) != 1 || loose[0].FactoryID != "LOOSE" {
		t.Fatalf("loose %+v", loose)
	}
	kept, err := st.ListDevicesForCar(ctx, keep)
	if err != nil {
		t.Fatal(err)
	}
	if len(kept) != 1 || kept[0].FactoryID != "KEEP" {
		t.Fatalf("must not steal KEEP: %+v", kept)
	}
	if atomic.LoadInt32(&asked) < 1 {
		t.Fatal("expected per-device VIN GET")
	}
}

func TestPairDevicesByVINDoesNotJoinDisplayName(t *testing.T) {
	p := filepath.Join(t.TempDir(), "name.sqlite")
	st, err := store.Open("sqlite", p)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	if err := st.UpsertCar(ctx, model.Car{EFleetsID: "27TESTA", VIN: "1HGCM82633A004352", Nickname: "WrongCar"}); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertDevice(ctx, model.OneStepDevice{
		FactoryID: "NOPE", DeviceID: "D", DisplayName: "WrongCar", Active: true,
	}); err != nil {
		t.Fatal(err)
	}
	a := &App{Store: st}
	res, err := a.PairDevicesByVIN(ctx, PairVINOpts{
		Live: []model.OneStepDevice{{FactoryID: "NOPE", DeviceID: "D", DisplayName: "WrongCar", Active: true}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Linked != 0 {
		t.Fatalf("display_name must not join: %+v", res)
	}
}

func TestApplyDeviceInformationPairsFromFileNoHTTP(t *testing.T) {
	p := filepath.Join(t.TempDir(), "info.sqlite")
	st, err := store.Open("sqlite", p)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	keep := "MAPPED"
	if err := st.UpsertCar(ctx, model.Car{EFleetsID: "27TESTA", VIN: "1HGCM82633A004352", Nickname: "WrongCar"}); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertCar(ctx, model.Car{EFleetsID: "OTHER", VIN: "1HGCM82633A000099", Nickname: "WrongCar"}); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertCar(ctx, model.Car{EFleetsID: keep, VIN: "1HGCM82633A000001"}); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertDevice(ctx, model.OneStepDevice{
		FactoryID: "KEEP", DeviceID: "OLD", LinkedCarEFleetsID: &keep, Active: true,
	}); err != nil {
		t.Fatal(err)
	}

	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		http.Error(w, "no live /device during file apply", http.StatusTooManyRequests)
	}))
	defer srv.Close()
	c := onestep.NewClient(srv.URL, "tok")
	c.HTTP = srv.Client()
	a := &App{Cfg: config.Config{OneStepToken: "tok"}, Store: st, OneStep: c}

	res, err := a.ApplyDeviceInformation(ctx, testdata("onestep", "device_information.json"))
	if err != nil {
		t.Fatal(err)
	}
	if atomic.LoadInt32(&hits) != 0 {
		t.Fatal("file apply must not GET /device")
	}
	if res.Asked != 0 {
		t.Fatalf("asked live %+v", res)
	}
	if res.Linked != 1 || len(res.Links) != 1 || res.Links[0].FactoryID != "FACTVIN" || res.Links[0].EFleetsID != "27TESTA" {
		t.Fatalf("OBD/report VIN must join unpaired box: %+v", res)
	}
	fact, err := st.ListDevicesForCar(ctx, "27TESTA")
	if err != nil {
		t.Fatal(err)
	}
	if len(fact) != 1 || fact[0].FactoryID != "FACTVIN" {
		t.Fatalf("FACTVIN %+v", fact)
	}
	kept, err := st.GetDevice(ctx, "KEEP")
	if err != nil || kept == nil || kept.LinkedCarEFleetsID == nil || *kept.LinkedCarEFleetsID != keep {
		t.Fatalf("existing factory_id map must not be stolen: %+v %v", kept, err)
	}
	nope, err := st.GetDevice(ctx, "NOPE")
	if err != nil || nope == nil || nope.LinkedCarEFleetsID != nil {
		t.Fatalf("display_name must not join: %+v %v", nope, err)
	}
	params, err := st.GetDevice(ctx, "PARAMSONLY")
	if err != nil || params == nil || params.LinkedCarEFleetsID != nil {
		t.Fatalf("params.vin must not join: %+v %v", params, err)
	}
}

func TestApplyDeviceInformationMissingFile(t *testing.T) {
	p := filepath.Join(t.TempDir(), "miss.sqlite")
	st, err := store.Open("sqlite", p)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	a := &App{Store: st}
	_, err = a.ApplyDeviceInformation(context.Background(), filepath.Join(t.TempDir(), "no-such-device-information.json"))
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("want honest missing-file error, got %v", err)
	}
	if os.IsNotExist(err) {
		t.Fatal("error should wrap the path for the operator, not be a bare os.ErrNotExist")
	}
}
