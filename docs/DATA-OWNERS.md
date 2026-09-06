# Who owns what (Neon vs Supabase vs SQLite)

No secrets in this file.

| Layer | System | Owns |
|-------|--------|------|
| Working store | **SQLite** (`OILCHANGE_DB`) | Ingest, compute, serve, desk login hashes, **encrypted vault**, places catalog, marker jobs, mileage ledger + drive-stop windows |
| Oil Desk publish | **Supabase** `ZacharyTFerguson's Project` (`hdtwfdjdvdzdxfdriyzn`) | `fleet_cars` (anon SELECT). Later: full sqlite-table copy if Neon is down |
| Backup | **Neon** `Fleet_Management_Neon` / `Fleet_Manage_Oil` (`icy-thunder-13848536`) | Unprefixed oilchange tables. Unpooled `DATABASE_URL` only |

Never write fleet oil to XRAY (`chjqcznyxvtjbamttqdj`).

Vault rows (`vault_secrets`) and `desk_users` stay on sqlite. They are **not** copied to Neon or Supabase. Places catalog and mileage ledger **are** copied to Neon (not the vault).

Connection health (no secret values): Oil Desk → **Status**.
