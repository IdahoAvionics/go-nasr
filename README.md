# go-nasr

Go library that converts an FAA NASR 28-day subscription into a SQLite database.

The FAA publishes aeronautical data for the National Airspace System (NAS) every 28 days as a zip file containing CSV files. This library reads that zip and produces a fully relational SQLite database with all 63 tables, typed columns, and foreign key relationships.

## Installation

```bash
go get github.com/IdahoAvionics/go-nasr
```

Requires Go 1.25 or later. No CGo required (uses a pure-Go SQLite driver).

## Usage

```go
package main

import (
	"log"

	"github.com/IdahoAvionics/go-nasr"
)

func main() {
	err := nasr.Extract(
		"28DaySubscription_Effective_2026-02-19.zip",
		"nasr.db",
	)
	if err != nil {
		log.Fatal(err)
	}
}
```

The input zip can be downloaded from the [FAA NASR subscription page](https://www.faa.gov/air_traffic/flight_info/aeronav/aero_data/NASR_Subscription/). The output database must not already exist.

## Foreign keys

The database defines 38 foreign key relationships between related tables within each data group (e.g., APT_RWY references APT_BASE on SITE_NO). Foreign key enforcement is off by default. To enable it:

```sql
PRAGMA foreign_keys = ON;
```

Cross-group foreign keys (e.g., ILS referencing APT) are intentionally omitted because the source data contains references to records that may not exist in other groups.

## What's in the database

The database contains 63 tables across 24 groups covering airports, navaids, fixes, airways, airspace, procedures, and more:

| Group | Tables | Description |
|-------|--------|-------------|
| APT | APT_BASE, APT_RWY, APT_RWY_END, APT_ARS, APT_ATT, APT_CON, APT_RMK | ~19,600 airports with runways, runway ends, arresting systems, attendance, contacts, remarks |
| ARB | ARB_BASE, ARB_SEG | ~38 ARTCC boundary segments |
| ATC | ATC_BASE, ATC_SVC, ATC_ATIS, ATC_RMK | ~3,600 ATC facilities with services, ATIS, remarks |
| AWOS | AWOS | ~2,600 automated weather observing systems |
| AWY | AWY_BASE, AWY_SEG_ALT | ~1,500 airways with segment altitudes |
| CDR | CDR | ~41,000 coded departure routes |
| CLS_ARSP | CLS_ARSP | ~960 class airspace areas (B, C, D, E) |
| COM | COM | ~1,800 communication outlets |
| DP | DP_BASE, DP_APT, DP_RTE | ~1,200 instrument departure procedures with airports, routes |
| FIX | FIX_BASE, FIX_CHRT, FIX_NAV | ~70,000 fixes with chart references, associated navaids |
| FRQ | FRQ | ~40,600 enroute communication frequencies |
| FSS | FSS_BASE, FSS_RMK | ~75 flight service stations with remarks |
| HPF | HPF_BASE, HPF_SPD_ALT, HPF_CHRT, HPF_RMK | ~15,700 preferred routes with speed/altitude, charts, remarks |
| ILS | ILS_BASE, ILS_GS, ILS_DME, ILS_MKR, ILS_RMK | ~1,600 ILS/LOC systems with glide slopes, DME, markers, remarks |
| LID | LID | ~31,200 location identifiers |
| MAA | MAA_BASE, MAA_SHP, MAA_RMK, MAA_CON | ~170 military airspace areas with shapes, remarks, contacts |
| MIL_OPS | MIL_OPS | ~200 military operations points |
| MTR | MTR_BASE, MTR_PT, MTR_AGY, MTR_SOP, MTR_TERR, MTR_WDTH | ~520 military training routes with points, agencies, SOPs, terrain, widths |
| NAV | NAV_BASE, NAV_CKPT, NAV_RMK | ~1,600 navaids (VOR, NDB, TACAN, etc.) with checkpoints, remarks |
| PFR | PFR_BASE, PFR_SEG, PFR_RMT_FMT | ~13,300 preferred routes with segments, remote formats |
| PJA | PJA_BASE, PJA_CON | ~690 parachute jump areas with contacts |
| RDR | RDR | ~370 radar facilities |
| STAR | STAR_BASE, STAR_APT, STAR_RTE | ~690 standard terminal arrival routes with airports, routes |
| WXL | WXL_BASE, WXL_SVC | ~3,400 weather locations with services |

Table schemas are derived at runtime from the FAA's own data structure definitions, so the library adapts automatically if the FAA adds or changes columns.

## Querying the database

```bash
sqlite3 nasr.db
```

```sql
-- Find an airport
SELECT ARPT_ID, ARPT_NAME, ICAO_ID, ELEV, LAT_DECIMAL, LONG_DECIMAL
FROM APT_BASE
WHERE ICAO_ID = 'KBOI';

-- List runways at an airport
SELECT r.RWY_ID, r.RWY_LEN, r.RWY_WIDTH, r.SURFACE_TYPE_CODE
FROM APT_RWY r
JOIN APT_BASE b ON r.SITE_NO = b.SITE_NO
WHERE b.ICAO_ID = 'KSEA';

-- Find VORs in Idaho
SELECT NAV_ID, NAME, FREQ, LAT_DECIMAL, LONG_DECIMAL
FROM NAV_BASE
WHERE STATE_CODE = 'ID' AND NAV_TYPE = 'VOR/DME';
```
