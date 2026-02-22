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

## What's in the database

The database contains 63 tables covering airports, navaids, fixes, airways, airspace, procedures, and more:

| Group | Tables | Example data |
|-------|--------|-------------|
| APT | APT_BASE, APT_RWY, APT_RWY_END, APT_ARS, APT_ATT, APT_CON, APT_RMK | ~19,000 airports with runways, contacts, remarks |
| NAV | NAV_BASE, NAV_CKPT, NAV_RMK | ~1,600 navaids (VOR, NDB, TACAN, etc.) |
| FIX | FIX_BASE, FIX_CHRT, FIX_NAV | ~70,000 fixes |
| AWY | AWY_BASE, AWY_SEG_ALT | Airways and segments |
| ILS | ILS_BASE, ILS_GS, ILS_DME, ILS_MKR, ILS_RMK | ILS/LOC systems |
| ATC | ATC_BASE, ATC_SVC, ATC_ATIS, ATC_RMK | ATC facilities |
| STAR/DP | STAR_BASE, STAR_APT, STAR_RTE, DP_BASE, DP_APT, DP_RTE | STARs and departure procedures |
| MTR | MTR_BASE, MTR_PT, MTR_AGY, MTR_SOP, MTR_TERR, MTR_WDTH | Military training routes |
| + 15 more groups | CDR, AWOS, COM, FSS, HPF, PFR, ARB, WXL, MAA, PJA, FRQ, LID, CLS_ARSP, MIL_OPS, RDR | |

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

## Foreign keys

The database defines 38 foreign key relationships between related tables within each data group (e.g., APT_RWY references APT_BASE on SITE_NO). Foreign key enforcement is off by default. To enable it:

```sql
PRAGMA foreign_keys = ON;
```

Cross-group foreign keys (e.g., ILS referencing APT) are intentionally omitted because the source data contains references to records that may not exist in other groups.
