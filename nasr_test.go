package nasr

import (
	"archive/zip"
	"bytes"
	"database/sql"
	"math"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

const testZipPath = "/Users/jacobmarble/projects/go-nasr/28DaySubscription_Effective_2026-02-19.zip"

var (
	testDBPath string
	testTmpDir string
)

func TestMain(m *testing.M) {
	if _, err := os.Stat(testZipPath); err == nil {
		dir, err := os.MkdirTemp("", "nasr-test-*")
		if err != nil {
			panic(err)
		}
		testTmpDir = dir
		testDBPath = filepath.Join(dir, "nasr.db")
		if err := Extract(testZipPath, testDBPath); err != nil {
			os.RemoveAll(dir)
			panic(err)
		}
	}

	code := m.Run()

	if testTmpDir != "" {
		os.RemoveAll(testTmpDir)
	}
	os.Exit(code)
}

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	if testDBPath == "" {
		t.Skip("NASR subscription zip not found")
	}
	db, err := sql.Open("sqlite", testDBPath)
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestExtract_MissingInput(t *testing.T) {
	dir := t.TempDir()
	err := Extract("/nonexistent/path/to.zip", filepath.Join(dir, "out.db"))
	if err == nil {
		t.Fatal("expected error for missing input file")
	}
}

func TestExtract_OutputExists(t *testing.T) {
	if _, err := os.Stat(testZipPath); err != nil {
		t.Skip("NASR subscription zip not found")
	}
	dir := t.TempDir()
	outPath := filepath.Join(dir, "out.db")
	if err := os.WriteFile(outPath, []byte("exists"), 0644); err != nil {
		t.Fatal(err)
	}
	err := Extract(testZipPath, outPath)
	if err == nil {
		t.Fatal("expected error when output file already exists")
	}
}

func TestExtract_TableCount(t *testing.T) {
	db := openTestDB(t)
	var count int
	err := db.QueryRow("SELECT count(*) FROM sqlite_master WHERE type='table'").Scan(&count)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if count != 63 {
		t.Errorf("expected 63 tables, got %d", count)
	}
}

func TestExtract_ColumnTypes(t *testing.T) {
	db := openTestDB(t)
	rows, err := db.Query("PRAGMA table_info(APT_BASE)")
	if err != nil {
		t.Fatalf("pragma: %v", err)
	}
	defer rows.Close()

	types := make(map[string]string)
	for rows.Next() {
		var cid int
		var name, colType string
		var notNull int
		var dfltValue *string
		var pk int
		if err := rows.Scan(&cid, &name, &colType, &notNull, &dfltValue, &pk); err != nil {
			t.Fatalf("scan: %v", err)
		}
		types[name] = colType
	}

	if types["LAT_DECIMAL"] != "REAL" {
		t.Errorf("LAT_DECIMAL type = %q, want REAL", types["LAT_DECIMAL"])
	}
	if types["ARPT_ID"] != "TEXT" {
		t.Errorf("ARPT_ID type = %q, want TEXT", types["ARPT_ID"])
	}
}

func TestExtract_ForeignKeys(t *testing.T) {
	db := openTestDB(t)
	rows, err := db.Query("PRAGMA foreign_key_list(APT_RWY)")
	if err != nil {
		t.Fatalf("pragma: %v", err)
	}
	defer rows.Close()

	found := false
	for rows.Next() {
		var id, seq int
		var table, from, to, onUpdate, onDelete, match string
		if err := rows.Scan(&id, &seq, &table, &from, &to, &onUpdate, &onDelete, &match); err != nil {
			t.Fatalf("scan: %v", err)
		}
		if table == "APT_BASE" && from == "SITE_NO" {
			found = true
		}
	}
	if !found {
		t.Error("expected FK from APT_RWY.SITE_NO to APT_BASE")
	}
}

func TestExtract_DataValues(t *testing.T) {
	db := openTestDB(t)
	var arptID, icaoID string
	var latDecimal float64
	err := db.QueryRow("SELECT ARPT_ID, ICAO_ID, LAT_DECIMAL FROM APT_BASE WHERE ARPT_ID='BOI'").
		Scan(&arptID, &icaoID, &latDecimal)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if icaoID != "KBOI" {
		t.Errorf("ICAO_ID = %q, want KBOI", icaoID)
	}
	if math.Abs(latDecimal-43.56) > 0.1 {
		t.Errorf("LAT_DECIMAL = %f, want ~43.56", latDecimal)
	}
}

func TestExtract_NullHandling(t *testing.T) {
	db := openTestDB(t)
	var count int
	err := db.QueryRow("SELECT count(*) FROM APT_BASE WHERE ICAO_ID IS NULL").Scan(&count)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if count == 0 {
		t.Error("expected some NULL ICAO_ID values in APT_BASE")
	}
}

func TestConvertValue(t *testing.T) {
	tests := []struct {
		name      string
		val       string
		col       columnDef
		tableName string
		want      interface{}
	}{
		{
			name: "empty nullable returns nil",
			val:  "", col: columnDef{name: "X", dataType: "TEXT", nullable: true},
			tableName: "TEST", want: nil,
		},
		{
			name: "empty non-nullable returns empty string",
			val:  "", col: columnDef{name: "X", dataType: "TEXT", nullable: false},
			tableName: "TEST", want: "",
		},
		{
			name: "REAL numeric converts to float",
			val:  "123.45", col: columnDef{name: "X", dataType: "REAL", nullable: false},
			tableName: "TEST", want: 123.45,
		},
		{
			name: "REAL non-numeric returns string",
			val:  "abc", col: columnDef{name: "X", dataType: "REAL", nullable: false},
			tableName: "TEST", want: "abc",
		},
		{
			name: "TEXT returns string",
			val:  "hello", col: columnDef{name: "X", dataType: "TEXT", nullable: false},
			tableName: "TEST", want: "hello",
		},
		{
			name: "REAL empty nullable returns nil",
			val:  "", col: columnDef{name: "X", dataType: "REAL", nullable: true},
			tableName: "TEST", want: nil,
		},
		{
			name: "sentinel NOT ASSIGNED becomes nil",
			val:  "NOT ASSIGNED", col: columnDef{name: "DP_COMPUTER_CODE", dataType: "TEXT", nullable: false},
			tableName: "DP_BASE", want: nil,
		},
		{
			name: "NOT ASSIGNED on non-sentinel column stays string",
			val:  "NOT ASSIGNED", col: columnDef{name: "OTHER", dataType: "TEXT", nullable: false},
			tableName: "DP_BASE", want: "NOT ASSIGNED",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := convertValue(tt.val, tt.col, tt.tableName)
			if got != tt.want {
				t.Errorf("convertValue(%q, %+v, %q) = %v (%T), want %v (%T)",
					tt.val, tt.col, tt.tableName, got, got, tt.want, tt.want)
			}
		})
	}
}

func TestParseSchemas(t *testing.T) {
	if _, err := os.Stat(testZipPath); err != nil {
		t.Skip("NASR subscription zip not found")
	}

	innerZip, _, err := openInnerCSVZip(testZipPath)
	if err != nil {
		t.Fatalf("openInnerCSVZip: %v", err)
	}

	tables, err := parseSchemas(innerZip)
	if err != nil {
		t.Fatalf("parseSchemas: %v", err)
	}
	if len(tables) != 63 {
		t.Errorf("expected 63 tables, got %d", len(tables))
	}
}

func TestGenerateDDL(t *testing.T) {
	tables := map[string]*tableSchema{
		"TEST_BASE": {
			name: "TEST_BASE",
			columns: []columnDef{
				{name: "ID", dataType: "TEXT", nullable: false},
				{name: "VALUE", dataType: "REAL", nullable: true},
			},
		},
		"TEST_CHILD": {
			name: "TEST_CHILD",
			columns: []columnDef{
				{name: "ID", dataType: "TEXT", nullable: false},
				{name: "BASE_ID", dataType: "TEXT", nullable: false},
			},
		},
	}
	fks := []foreignKey{
		{childTable: "TEST_CHILD", columns: []string{"BASE_ID"}, parentTable: "TEST_BASE"},
	}

	createTables, createIndexes := generateDDL(tables, fks)
	if len(createTables) != 2 {
		t.Fatalf("expected 2 CREATE TABLE statements, got %d", len(createTables))
	}
	if len(createIndexes) != 1 {
		t.Fatalf("expected 1 CREATE UNIQUE INDEX statement, got %d", len(createIndexes))
	}

	// Verify by executing against an in-memory SQLite database.
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	for _, stmt := range createTables {
		if _, err := db.Exec(stmt); err != nil {
			t.Errorf("exec DDL failed: %v\n%s", err, stmt)
		}
	}
	for _, stmt := range createIndexes {
		if _, err := db.Exec(stmt); err != nil {
			t.Errorf("exec index failed: %v\n%s", err, stmt)
		}
	}

	// Verify the foreign key was created.
	rows, err := db.Query("PRAGMA foreign_key_list(TEST_CHILD)")
	if err != nil {
		t.Fatalf("pragma: %v", err)
	}
	defer rows.Close()

	found := false
	for rows.Next() {
		var id, seq int
		var table, from, to, onUpdate, onDelete, match string
		if err := rows.Scan(&id, &seq, &table, &from, &to, &onUpdate, &onDelete, &match); err != nil {
			t.Fatalf("scan: %v", err)
		}
		if table == "TEST_BASE" && from == "BASE_ID" {
			found = true
		}
	}
	if !found {
		t.Error("expected FK from TEST_CHILD.BASE_ID to TEST_BASE")
	}
}

func TestExtract_UniqueIndexes(t *testing.T) {
	db := openTestDB(t)
	var count int
	err := db.QueryRow("SELECT count(*) FROM sqlite_master WHERE type='index' AND name LIKE 'idx_%'").Scan(&count)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if count != 18 {
		t.Errorf("expected 18 unique indexes, got %d", count)
	}

	// Verify NAV_BASE has a unique index (composite key we fixed).
	rows, err := db.Query("PRAGMA index_info(idx_NAV_BASE_NAV_ID_NAV_TYPE_CITY_COUNTRY_CODE)")
	if err != nil {
		t.Fatalf("pragma: %v", err)
	}
	defer rows.Close()
	found := false
	for rows.Next() {
		found = true
	}
	if !found {
		t.Error("expected unique index on NAV_BASE")
	}
}

func TestExtract_ForeignKeyCheck(t *testing.T) {
	db := openTestDB(t)
	rows, err := db.Query("PRAGMA foreign_key_check")
	if err != nil {
		t.Fatalf("foreign_key_check: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var table, rowid, parent, fkid string
		if err := rows.Scan(&table, &rowid, &parent, &fkid); err != nil {
			t.Fatalf("scan: %v", err)
		}
		t.Errorf("FK violation: table=%s rowid=%s parent=%s fkid=%s", table, rowid, parent, fkid)
	}
}

func TestExtract_DPNotAssignedIsNull(t *testing.T) {
	db := openTestDB(t)
	var count int
	err := db.QueryRow("SELECT count(*) FROM DP_BASE WHERE DP_COMPUTER_CODE IS NULL").Scan(&count)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if count == 0 {
		t.Error("expected NULL DP_COMPUTER_CODE values (converted from 'NOT ASSIGNED')")
	}
}

// TestOpenInnerCSVZip verifies that the real zip contains the expected inner CSV zip.
func TestOpenInnerCSVZip(t *testing.T) {
	if _, err := os.Stat(testZipPath); err != nil {
		t.Skip("NASR subscription zip not found")
	}

	zr, data, err := openInnerCSVZip(testZipPath)
	if err != nil {
		t.Fatalf("openInnerCSVZip: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("inner zip data is empty")
	}
	if len(zr.File) == 0 {
		t.Fatal("inner zip has no files")
	}

	// Verify we can create a new reader from the data.
	zr2, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("re-create reader: %v", err)
	}
	if len(zr2.File) != len(zr.File) {
		t.Errorf("re-created reader has %d files, original has %d", len(zr2.File), len(zr.File))
	}
}
