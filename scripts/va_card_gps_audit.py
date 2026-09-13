#!/usr/bin/env python3
"""VA card ↔ GPS sit audit from sqlite + gps-stops cache. Never writes Last Reading."""

from __future__ import annotations

import csv
import json
import math
import os
import sqlite3
from collections import Counter, defaultdict
from datetime import datetime, timezone, timedelta

DB = os.environ.get("OILCHANGE_DB", "./oilchange.sqlite")
CACHE = os.environ.get("GPS_STOPS", "data/runtime/gps-stops.json")
CARDS_JSON = os.environ.get("CARDS_JSON", "web/data/cards.json")
OUT = os.environ.get("AUDIT_OUT", "data/runtime/va-card-gps-audit.json")
SLACK_MIN = 20
PUMP_M = 350.0
MILE_M = 1609.344


def haversine_m(lat1, lng1, lat2, lng2) -> float:
    r = 6371000.0
    p1, p2 = math.radians(lat1), math.radians(lat2)
    dphi = math.radians(lat2 - lat1)
    dl = math.radians(lng2 - lng1)
    a = math.sin(dphi / 2) ** 2 + math.cos(p1) * math.cos(p2) * math.sin(dl / 2) ** 2
    return 2 * r * math.asin(min(1.0, math.sqrt(a)))


def parse_rfc3339(s: str) -> datetime | None:
    if not s:
        return None
    s = s.strip()
    if s.endswith("Z"):
        s = s[:-1] + "+00:00"
    try:
        return datetime.fromisoformat(s)
    except ValueError:
        return None


def main() -> None:
    con = sqlite3.connect(DB)
    con.row_factory = sqlite3.Row
    cars = list(con.execute("SELECT efleets_id, nickname, region, vin FROM cars"))
    va_ids = set()
    nick = {}
    for c in cars:
        eid = (c["efleets_id"] or "").strip()
        n = (c["nickname"] or "").strip()
        nick[eid] = n
        region = (c["region"] or "").strip().upper()
        if region == "VA" or n.upper().replace(" ", "").startswith("VA"):
            va_ids.add(eid)
    devices = list(
        con.execute(
            "SELECT factory_id, device_id, linked_car_efleets_id FROM onestep_devices"
        )
    )
    linked_cars = {
        (d["linked_car_efleets_id"] or "").strip()
        for d in devices
        if (d["linked_car_efleets_id"] or "").strip()
    }
    fid_to_car = {
        (d["factory_id"] or "").strip(): (d["linked_car_efleets_id"] or "").strip()
        for d in devices
        if (d["factory_id"] or "").strip()
    }
    txs = list(
        con.execute(
            """SELECT card_id, at, station_name, station_address, recorded_efleets_id
               FROM card_transactions"""
        )
    )
    eras = list(con.execute("SELECT card_id, holder_type, holder_key, efleets_id, from_at, to_at, evidence_n, split FROM card_eras"))
    lr = con.execute("SELECT COUNT(*) n FROM cars WHERE last_reading_miles IS NOT NULL").fetchone()["n"]

    visits = []
    if os.path.exists(CACHE):
        blob = json.load(open(CACHE))
        visits = blob.get("visits") or []

    geo_by_key = {}
    if os.path.exists(CARDS_JSON):
        snap = json.load(open(CARDS_JSON))
        for g in snap.get("geocoded_stations") or []:
            k = (g.get("key") or "").strip().lower()
            if k and g.get("lat") is not None:
                geo_by_key[k] = g

    va_txs = [t for t in txs if (t["recorded_efleets_id"] or "").strip() in va_ids]
    by_card = defaultdict(list)
    for t in va_txs:
        by_card[t["card_id"]].append(t)

    def station_key(name, addr):
        n = (name or "").strip().upper()
        a = (addr or "").strip().upper()
        if n in ("TRACKER", ""):
            return ""
        return f"{n}|{a}"

    def geo_key(name, addr):
        n = (name or "").strip().lower()
        a = (addr or "").strip()
        if not n or n in ("tracker", "unknown"):
            return ""
        parts = [p.strip() for p in a.split(",") if p.strip()]
        city = parts[-2].strip().lower() if len(parts) >= 2 else ""
        st = parts[-1].strip().lower()[:2] if parts else ""
        if city and st:
            return f"{n}|{city}|{st}"
        return ""

    slack = timedelta(minutes=SLACK_MIN)
    pos_visits = [v for v in visits if v.get("has_pos") and v.get("factory_id")]

    card_reports = []
    switches = []
    aliases = Counter()
    unknown = []

    for card, rows in sorted(by_card.items(), key=lambda kv: -len(kv[1])):
        rows = sorted(rows, key=lambda r: r["at"] or "", reverse=True)
        cars_on_card = Counter((r["recorded_efleets_id"] or "").strip() for r in rows)
        loc_seen = []
        loc_keys = set()
        for r in rows:
            k = station_key(r["station_name"], r["station_address"])
            if not k or k in loc_keys:
                continue
            loc_keys.add(k)
            loc_seen.append(r)
            if len(loc_seen) == 3:
                break
        loc_results = []
        exclusive_cars = Counter()
        for r in loc_seen:
            at = parse_rfc3339(r["at"])
            if at is None:
                continue
            at = at.astimezone(timezone.utc)
            gk = geo_key(r["station_name"], r["station_address"])
            geo = geo_by_key.get(gk)
            cands = []
            for v in pos_visits:
                start = parse_rfc3339(v.get("from") or "")
                end = parse_rfc3339(v.get("to") or "")
                if start is None:
                    continue
                if end is None:
                    end = start
                if start.tzinfo is None:
                    start = start.replace(tzinfo=timezone.utc)
                if end.tzinfo is None:
                    end = end.replace(tzinfo=timezone.utc)
                if end < at - slack or start > at + slack:
                    continue
                dur = end - start
                if dur > timedelta(hours=2):
                    continue  # overnight sit
                lat, lng = v.get("lat"), v.get("lng")
                if geo and lat is not None and lng is not None:
                    if haversine_m(float(lat), float(lng), float(geo["lat"]), float(geo["lng"])) > PUMP_M:
                        continue
                elif geo is None:
                    continue  # no pump coords → do not count fleet-wide time sits
                car = fid_to_car.get((v.get("factory_id") or "").strip(), "")
                cands.append(
                    {
                        "factory_id": v.get("factory_id"),
                        "car": car,
                        "nick": nick.get(car, ""),
                        "lat": lat,
                        "lng": lng,
                    }
                )
            by_car = {}
            for c in cands:
                if c["car"]:
                    by_car[c["car"]] = c
            loc_results.append(
                {
                    "at": r["at"],
                    "station": (r["station_name"] or "").strip(),
                    "address": (r["station_address"] or "").strip(),
                    "recorded": (r["recorded_efleets_id"] or "").strip(),
                    "geocoded": bool(geo),
                    "cars_at_pump_time": sorted(by_car.keys()),
                    "n_cars": len(by_car),
                    "exclusive": list(by_car.keys())[0] if len(by_car) == 1 else None,
                }
            )
            if len(by_car) == 1:
                exclusive_cars[list(by_car.keys())[0]] += 1
            brand = (r["station_name"] or "").strip().upper()
            if brand:
                aliases[brand] += 1
        gps_car = exclusive_cars.most_common(1)[0][0] if exclusive_cars else None
        rec_car = cars_on_card.most_common(1)[0][0] if cars_on_card else ""
        if gps_car and rec_car and gps_car != rec_car:
            switches.append(
                {
                    "card": card,
                    "recorded": rec_car,
                    "gps": gps_car,
                    "evidence": dict(exclusive_cars),
                }
            )
        weak = gps_car is None
        if weak:
            unknown.append(card)
        card_reports.append(
            {
                "card": card,
                "punches": len(rows),
                "recorded_cars": cars_on_card.most_common(),
                "gps_exclusive_car": gps_car,
                "three_locations": loc_results,
            }
        )

    roster = len(cars)
    device_link = len({c["efleets_id"] for c in cars if (c["efleets_id"] or "").strip() in linked_cars})
    va_n = len(va_ids)
    va_linked = len(va_ids & linked_cars)
    era_cars = {
        (e["efleets_id"] or e["holder_key"] or "").strip()
        for e in eras
        if (e["holder_type"] or "car") == "car"
    }
    va_era = len(va_ids & era_cars)
    known = len(linked_cars & era_cars)

    out = {
        "generated_at": datetime.now(timezone.utc).isoformat(),
        "last_reading_written": lr,
        "roster": roster,
        "device_link_n": device_link,
        "device_link_pct": round(100.0 * device_link / roster, 1) if roster else 0,
        "card_era_n": len(era_cars),
        "known_n": known,
        "known_pct": round(100.0 * known / roster, 1) if roster else 0,
        "va_cars": va_n,
        "va_device_link": va_linked,
        "va_card_era": va_era,
        "va_cards": len(by_card),
        "va_cards_gps_exclusive": len(by_card) - len(unknown),
        "gps_stops_visits": len(visits),
        "gps_stops_boxes": len({v.get("factory_id") for v in visits if v.get("factory_id")}),
        "geocoded_stations_n": len(geo_by_key),
        "brand_aliases": aliases.most_common(),
        "switches": switches,
        "unknown_cards": unknown,
        "cards": card_reports,
        "missing_device": sorted(
            (c["efleets_id"] or "") + " " + (c["nickname"] or "")
            for c in cars
            if (c["efleets_id"] or "").strip() not in linked_cars
        ),
        "note": "GPS sits are VA watched boxes in gps-stops.json, not a 260-box nearby live pull. Exclusive among cached boxes only.",
    }
    os.makedirs(os.path.dirname(OUT) or ".", exist_ok=True)
    with open(OUT, "w") as f:
        json.dump(out, f, indent=2)
    print(
        f"roster={roster} device_link={device_link}/{roster} ({out['device_link_pct']}%) "
        f"known={known}/{roster} ({out['known_pct']}%) "
        f"VA cars={va_n} linked={va_linked} gps_exclusive_cards={out['va_cards_gps_exclusive']}/{len(by_card)} "
        f"switches={len(switches)} last_reading={lr} boxes={out['gps_stops_boxes']} visits={len(visits)}"
    )
    print("unknown cards", len(unknown))
    for s in switches:
        print("SWITCH", s)
    print("wrote", OUT)


if __name__ == "__main__":
    main()
