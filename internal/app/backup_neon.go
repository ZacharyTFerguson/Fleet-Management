package app

import (
	"context"
	"fmt"
	"strings"

	"oilchange/internal/store"
)

// BackupCounts is how many source rows the Neon/sqlite copier processed.
type BackupCounts struct {
	Cars, Fills, ShopROs, Stations, MaintLocs int
	Cards, CardTxs, Pairings                  int
	Devices, Miles, Holds, OilChanges         int
}

// LogLine is the operator-facing summary. It never includes a DSN or password.
func (c BackupCounts) LogLine() string {
	return fmt.Sprintf("neon backup cars=%d fills=%d shop_ros=%d holds=%d oil_changes=%d devices=%d miles=%d cards=%d card_txs=%d pairings=%d stations=%d maint_locs=%d",
		c.Cars, c.Fills, c.ShopROs, c.Holds, c.OilChanges, c.Devices, c.Miles, c.Cards, c.CardTxs, c.Pairings, c.Stations, c.MaintLocs)
}

// validateNeonBackupURL refuses empty, pooled, XRAY, and Supabase DATABASE_URL values.
// Backup writes must use a Neon unpooled URL, not cfg.DSN() (which stays sqlite).
func validateNeonBackupURL(url string) error {
	if strings.TrimSpace(url) == "" {
		return fmt.Errorf("set DATABASE_URL in oilchange.env to the Neon unpooled connection string (hostname without -pooler)")
	}
	lu := strings.ToLower(url)
	if strings.Contains(lu, "chjqcznyxvtjbamttqdj") || strings.Contains(lu, "xray") {
		return fmt.Errorf("refusing XRAY Supabase project for Neon backup")
	}
	if strings.Contains(lu, "supabase.co") || strings.Contains(lu, "supabase.com") {
		return fmt.Errorf("DATABASE_URL is a Supabase host; set it to Neon unpooled (not SUPABASE_URL)")
	}
	if strings.Contains(lu, "-pooler") {
		return fmt.Errorf("DATABASE_URL looks pooled (-pooler); use Neon unpooled (DATABASE_URL_UNPOOLED) for backup")
	}
	if !strings.Contains(lu, "neon.tech") {
		return fmt.Errorf("DATABASE_URL must be a Neon unpooled URL")
	}
	return nil
}

// BackupNeon copies durable sqlite rows into Neon Postgres.
// Source is a.Store (sqlite via cfg.DSN). Dest is a second pgx store from DATABASE_URL.
// Does not run compute and does not write Last Reading except as copied cars columns.
func (a *App) BackupNeon(ctx context.Context) (BackupCounts, error) {
	if err := validateNeonBackupURL(a.Cfg.DatabaseURL); err != nil {
		return BackupCounts{}, err
	}
	dest, err := store.Open("pgx", a.Cfg.DatabaseURL)
	if err != nil {
		return BackupCounts{}, fmt.Errorf("open neon: %w", err)
	}
	defer dest.Close()
	return CopyDurable(ctx, a.Store, dest)
}

// CopyDurable copies oilchange tables from src to dest. Dest may be sqlite (tests) or pgx (Neon).
// HOLD state and last_reading_* are copied as stored; this path does not invent miles.
func CopyDurable(ctx context.Context, src, dest *store.Store) (BackupCounts, error) {
	var c BackupCounts

	cars, err := src.ListCars(ctx)
	if err != nil {
		return c, fmt.Errorf("list cars: %w", err)
	}
	for _, car := range cars {
		if err := dest.UpsertCar(ctx, car); err != nil {
			return c, fmt.Errorf("upsert car %s: %w", car.EFleetsID, err)
		}
		if err := dest.ApplyCarSnapshot(ctx, car); err != nil {
			return c, fmt.Errorf("snapshot car %s: %w", car.EFleetsID, err)
		}
	}
	c.Cars = len(cars)

	stations, err := src.ListStations(ctx)
	if err != nil {
		return c, fmt.Errorf("list stations: %w", err)
	}
	for _, g := range stations {
		if err := dest.UpsertStation(ctx, g); err != nil {
			return c, fmt.Errorf("upsert station %s: %w", g.ID, err)
		}
	}
	c.Stations = len(stations)

	locs, err := src.ListMaintLocs(ctx)
	if err != nil {
		return c, fmt.Errorf("list maint locs: %w", err)
	}
	for _, m := range locs {
		if err := dest.UpsertMaintLoc(ctx, m); err != nil {
			return c, fmt.Errorf("upsert maint loc %s: %w", m.ID, err)
		}
	}
	c.MaintLocs = len(locs)

	cards, err := src.ListCards(ctx)
	if err != nil {
		return c, fmt.Errorf("list cards: %w", err)
	}
	for _, card := range cards {
		if err := dest.UpsertCard(ctx, card); err != nil {
			return c, fmt.Errorf("upsert card %s: %w", card.ID, err)
		}
	}
	c.Cards = len(cards)

	fills, err := src.ListAllFills(ctx)
	if err != nil {
		return c, fmt.Errorf("list fills: %w", err)
	}
	for _, f := range fills {
		if err := dest.UpsertFill(ctx, f); err != nil {
			return c, fmt.Errorf("upsert fill %s: %w", f.EFleetsID, err)
		}
	}
	c.Fills = len(fills)

	ros, err := src.ListAllShopROs(ctx)
	if err != nil {
		return c, fmt.Errorf("list shop ros: %w", err)
	}
	for _, r := range ros {
		if err := dest.UpsertShopRO(ctx, r); err != nil {
			return c, fmt.Errorf("upsert shop ro %s: %w", r.ROID, err)
		}
	}
	c.ShopROs = len(ros)

	oils, err := src.ListOilChanges(ctx)
	if err != nil {
		return c, fmt.Errorf("list oil changes: %w", err)
	}
	for _, o := range oils {
		ok, err := dest.HasOilChange(ctx, o.EFleetsID, o.Miles, o.Date)
		if err != nil {
			return c, fmt.Errorf("has oil change %s: %w", o.EFleetsID, err)
		}
		if ok {
			continue
		}
		if err := dest.InsertOilChangeRow(ctx, o); err != nil {
			return c, fmt.Errorf("insert oil change %s: %w", o.EFleetsID, err)
		}
	}
	c.OilChanges = len(oils)

	holds, err := src.ListHoldEvents(ctx)
	if err != nil {
		return c, fmt.Errorf("list holds: %w", err)
	}
	for _, h := range holds {
		ok, err := dest.HasHoldEvent(ctx, h)
		if err != nil {
			return c, fmt.Errorf("has hold %s: %w", h.EFleetsID, err)
		}
		if ok {
			continue
		}
		if err := dest.InsertHoldEvent(ctx, h); err != nil {
			return c, fmt.Errorf("insert hold %s: %w", h.EFleetsID, err)
		}
	}
	c.Holds = len(holds)

	devs, err := src.ListDevices(ctx)
	if err != nil {
		return c, fmt.Errorf("list devices: %w", err)
	}
	for _, d := range devs {
		if err := dest.UpsertDevice(ctx, d); err != nil {
			return c, fmt.Errorf("upsert device %s: %w", d.FactoryID, err)
		}
	}
	c.Devices = len(devs)

	miles, err := src.ListAllMilesSince(ctx)
	if err != nil {
		return c, fmt.Errorf("list miles: %w", err)
	}
	for _, m := range miles {
		if err := dest.SaveMilesSince(ctx, m); err != nil {
			return c, fmt.Errorf("save miles %s: %w", m.FactoryID, err)
		}
	}
	c.Miles = len(miles)

	txs, err := src.ListCardTxs(ctx, "")
	if err != nil {
		return c, fmt.Errorf("list card txs: %w", err)
	}
	for _, t := range txs {
		if err := dest.UpsertCardTx(ctx, t); err != nil {
			return c, fmt.Errorf("upsert card tx %s: %w", t.CardID, err)
		}
	}
	c.CardTxs = len(txs)

	pairs, err := src.ListPairings(ctx, "")
	if err != nil {
		return c, fmt.Errorf("list pairings: %w", err)
	}
	if err := dest.ReplacePairings(ctx, pairs); err != nil {
		return c, fmt.Errorf("replace pairings: %w", err)
	}
	c.Pairings = len(pairs)
	return c, nil
}
