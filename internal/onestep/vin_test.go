package onestep

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"oilchange/internal/model"
)

func TestAskDeviceVINUsesOBDNotDisplayName(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("device_id") != "DEVVIN" {
			t.Errorf("device_id %s", r.URL.Query().Get("device_id"))
		}
		if r.URL.Query().Get("latest_point") != "true" {
			t.Errorf("latest_point %s", r.URL.Query().Get("latest_point"))
		}
		if r.URL.Query().Get("factory_id") != "" {
			t.Error("must not send factory_id")
		}
		_, _ = w.Write([]byte(`[{
			"factory_id":"FACTVIN","device_id":"DEVVIN","display_name":"WrongCar",
			"latest_device_point":{"device_state":{"vin":"1HGCM82633A004352"},"params":{"vin":"IGNOREDPARAMS"}}
		}]`))
	}))
	defer srv.Close()
	c := NewClient(srv.URL, "tok")
	c.HTTP = srv.Client()
	vin, all, err := c.AskDeviceVIN(t.Context(), model.OneStepDevice{FactoryID: "FACTVIN", DeviceID: "DEVVIN", DisplayName: "WrongCar"})
	if err != nil {
		t.Fatal(err)
	}
	if vin != "1HGCM82633A004352" {
		t.Fatalf("vin %q all=%+v", vin, all)
	}
}

func TestAskDeviceVINIgnoresParamsVIN(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[{
			"factory_id":"FACTVIN","device_id":"DEVVIN","display_name":"WrongCar",
			"latest_device_point":{"params":{"vin":"1HGCM82633A004352"}}
		}]`))
	}))
	defer srv.Close()
	c := NewClient(srv.URL, "tok")
	c.HTTP = srv.Client()
	vin, _, err := c.AskDeviceVIN(t.Context(), model.OneStepDevice{FactoryID: "FACTVIN", DeviceID: "DEVVIN"})
	if err != nil {
		t.Fatal(err)
	}
	if vin != "" {
		t.Fatalf("params.vin is not identity: %q", vin)
	}
}

func TestValidVINRejectsShort(t *testing.T) {
	if ValidVIN("SHORT") != "" {
		t.Fatal("short")
	}
	if ValidVIN(" 1hgcm82633a004352 ") != "1HGCM82633A004352" {
		t.Fatal("normalize")
	}
	if strings.Contains(ValidVIN("WrongCar"), "W") && len(ValidVIN("WrongCar")) == 17 {
		t.Fatal("display_name")
	}
}
