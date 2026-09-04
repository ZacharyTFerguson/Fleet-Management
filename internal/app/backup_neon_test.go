package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"oilchange/internal/model"
	"oilchange/internal/store"
)

func TestValidateNeonBackupURL(t *testing.T) {
	if err := validateNeonBackupURL(""); err == nil {
		t.Fatal("empty")
	}
	if err := validateNeonBackupURL("postgres://u:p@db.chjqcznyxvtjbamttqdj.supabase.co/postgres"); err == nil {
		t.Fatal("xray")
	}
	if err := validateNeonBackupURL("postgres://u:p@db.hdtwfdjdvdzdxfdriyzn.supabase.co/postgres"); err == nil {
		t.Fatal("supabase")
	}
	if err := validateNeonBackupURL("postgres://u:p@ep-raspy-moon-a52206a2-pooler.us-east-2.aws.neon.tech/db"); err == nil {
		t.Fatal("pooler")
	}
	if err := validateNeonBackupURL("postgres://u:p@localhost:5432/db"); err == nil {
		t.Fatal("not neon")
	}
	if err := validateNeonBackupURL("postgres://u:p@ep-raspy-moon-a52206a2.us-east-2.aws.neon.tech/Fleet_Manage_Oil?sslmode=require"); err != nil {
		t.Fatal(err)
	}
}

func TestCopyDurableSQLiteToSQLite(t *testing.T) {
	ctx := context.Background()
	src, err := store.Open("sqlite", filepath.Join(t.TempDir(), "src.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()
	dest, err := store.Open("sqlite", filepath.Join(t.TempDir(), "dest.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer dest.Close()

	odo := 100000
	at := time.Date(2026, 8, 1, 15, 4, 5, 0, time.UTC)
	if err := src.UpsertCar(ctx, model.Car{PDIID: "PDI-0042", EFleetsID: "27TESTA", Nickname: "VA19", IntervalMiles: 5000}); err != nil {
		t.Fatal(err)
	}
	if err := src.UpsertFill(ctx, model.Fill{EFleetsID: "27TESTA", Odometer: &odo, ProviderTransactionTime: at, Source: model.SourceFuelDetails, MerchantName: "Shell"}); err != nil {
		t.Fatal(err)
	}
	if err := src.UpsertStation(ctx, model.GasStation{ID: "st1", Name: "Shell", Address: "1 Main"}); err != nil {
		t.Fatal(err)
	}
	if err := src.UpsertMaintLoc(ctx, model.MaintenanceLocation{ID: "ml1", Name: "PDI Shop"}); err != nil {
		t.Fatal(err)
	}
	if err := src.UpsertShopRO(ctx, model.ShopRO{ROID: "RO1", EFleetsID: "27TESTA", Odometer: 100500, At: at.Add(24 * time.Hour), LocationName: "PDI Shop", ServiceDesc: "oil/lube"}); err != nil {
		t.Fatal(err)
	}
	if err := src.InsertOilChange(ctx, model.OilChange{EFleetsID: "27TESTA", Miles: 100500, Date: at, Location: "PDI Shop", Source: "shop_ro"}); err != nil {
		t.Fatal(err)
	}
	if err := src.WriteLastReading(ctx, "27TESTA", 100010, at, model.SourceFuelDetails); err != nil {
		t.Fatal(err)
	}
	if err := src.SetHold(ctx, "27TESTA", model.HoldUnusualY, "copied as-is"); err != nil {
		t.Fatal(err)
	}
	link := "27TESTA"
	if err := src.UpsertDevice(ctx, model.OneStepDevice{FactoryID: "FACT1", DeviceID: "dev1", DisplayName: "label-only", LinkedCarEFleetsID: &link, Active: true}); err != nil {
		t.Fatal(err)
	}
	if err := src.SaveMilesSince(ctx, model.DriveStopMiles{FactoryID: "FACT1", Since: at, Miles: 12.5}); err != nil {
		t.Fatal(err)
	}
	if err := src.UpsertCard(ctx, model.Card{ID: "CARD1", CompanyVehicleNumber: "CVN1", LinkedCarEFleetsID: &link}); err != nil {
		t.Fatal(err)
	}
	if err := src.UpsertCardTx(ctx, model.CardTx{CardID: "CARD1", At: at, StationName: "Shell", RecordedEFleetsID: "27TESTA", Odometer: &odo}); err != nil {
		t.Fatal(err)
	}
	if err := src.ReplacePairings(ctx, []model.CardPairing{{CardID: "CARD1", EntityType: "car", EntityKey: "27TESTA", EvidenceN: 1, Score: 1, Best: true}}); err != nil {
		t.Fatal(err)
	}

	counts, err := CopyDurable(ctx, src, dest)
	if err != nil {
		t.Fatal(err)
	}
	if counts.Cars != 1 || counts.Fills != 1 || counts.Holds != 1 || counts.Devices != 1 || counts.Miles != 1 {
		t.Fatalf("counts %+v", counts)
	}
	if !strings.Contains(counts.LogLine(), "cars=1") || strings.Contains(counts.LogLine(), "postgres://") {
		t.Fatalf("log %q", counts.LogLine())
	}

	got, err := dest.CarByEFleets(ctx, "27TESTA")
	if err != nil {
		t.Fatal(err)
	}
	if got.PDIID != "PDI-0042" {
		t.Fatalf("pdi %s", got.PDIID)
	}
	if got.LastReadingMiles == nil || *got.LastReadingMiles != 100010 {
		t.Fatalf("last reading %+v", got.LastReadingMiles)
	}
	if got.HoldReason == nil || *got.HoldReason != model.HoldUnusualY {
		t.Fatalf("hold must be copied, not cleared: %+v", got.HoldReason)
	}
	if got.LastOilMiles == nil || *got.LastOilMiles != 100500 {
		t.Fatalf("last oil %+v", got.LastOilMiles)
	}

	fills, err := dest.ListFills(ctx, "27TESTA")
	if err != nil || len(fills) != 1 {
		t.Fatalf("fills %d %v", len(fills), err)
	}
	holds, err := dest.ListHoldEvents(ctx)
	if err != nil || len(holds) != 1 || !holds[0].Open {
		t.Fatalf("holds %+v %v", holds, err)
	}
	devs, err := dest.ListDevices(ctx)
	if err != nil || len(devs) != 1 || devs[0].FactoryID != "FACT1" {
		t.Fatalf("devices %+v %v", devs, err)
	}

	again, err := CopyDurable(ctx, src, dest)
	if err != nil {
		t.Fatal(err)
	}
	if again.Cars != 1 {
		t.Fatalf("second copy %+v", again)
	}
	holds, err = dest.ListHoldEvents(ctx)
	if err != nil || len(holds) != 1 {
		t.Fatalf("hold events duplicated: %d %v", len(holds), err)
	}
	oils, err := dest.ListOilChanges(ctx)
	if err != nil || len(oils) != 1 {
		t.Fatalf("oil changes duplicated: %d %v", len(oils), err)
	}
}

func TestBackupNeonIntegration(t *testing.T) {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL not set")
	}
	if err := validateNeonBackupURL(url); err != nil {
		t.Skip(err.Error())
	}
	dest, err := store.Open("pgx", url)
	if err != nil {
		t.Fatal(err)
	}
	defer dest.Close()
	if _, err := dest.ListCars(context.Background()); err != nil {
		t.Fatal(err)
	}
}
