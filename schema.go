package nasr

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
	"fmt"
	"io"
	"sort"
	"strings"
)

type columnDef struct {
	name     string
	dataType string
	nullable bool
}

type tableSchema struct {
	name    string
	columns []columnDef
}

// parseSchemas reads all *_CSV_DATA_STRUCTURE.csv files from the zip and returns
// a map of table name to schema. There are 24 structure files defining 63 tables.
func parseSchemas(zr *zip.Reader) (map[string]*tableSchema, error) {
	tables := make(map[string]*tableSchema)

	for _, f := range zr.File {
		if !strings.HasSuffix(f.Name, "_CSV_DATA_STRUCTURE.csv") {
			continue
		}

		rc, err := f.Open()
		if err != nil {
			return nil, fmt.Errorf("open %s: %w", f.Name, err)
		}

		raw, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", f.Name, err)
		}

		// Some files (notably FSS_CSV_DATA_STRUCTURE.csv) use bare CR (0x0d)
		// line endings instead of CRLF. Go's csv.Reader does not handle bare
		// CR as a line terminator. Replace bare \r with \n.
		raw = normalizeCR(raw)

		r := csv.NewReader(bytes.NewReader(raw))
		r.FieldsPerRecord = -1 // allow variable field count

		// Read and discard header row.
		if _, err := r.Read(); err != nil {
			return nil, fmt.Errorf("read header in %s: %w", f.Name, err)
		}

		for {
			record, err := r.Read()
			if err == io.EOF {
				break
			}
			if err != nil {
				return nil, fmt.Errorf("parse %s: %w", f.Name, err)
			}
			if len(record) < 5 {
				continue
			}

			tableName := strings.TrimSpace(record[0])
			colName := strings.TrimSpace(record[1])
			// record[2] is Max Length (unused)
			dataType := strings.TrimSpace(record[3])
			nullableStr := strings.TrimSpace(record[4])

			colName = strings.ReplaceAll(colName, " ", "_")

			switch dataType {
			case "VARCHAR":
				dataType = "TEXT"
			case "NUMBER":
				dataType = "REAL"
			}

			nullable := nullableStr == "Yes"

			ts, ok := tables[tableName]
			if !ok {
				ts = &tableSchema{name: tableName}
				tables[tableName] = ts
			}
			ts.columns = append(ts.columns, columnDef{
				name:     colName,
				dataType: dataType,
				nullable: nullable,
			})
		}
	}

	// Override nullability for columns with sentinel NULL values.
	// These columns are marked NOT NULL in the FAA schema but contain
	// placeholder values (e.g., "NOT ASSIGNED") that we convert to NULL.
	for key := range sentinelNulls {
		tableName, colName := key[0], key[1]
		if ts, ok := tables[tableName]; ok {
			for i := range ts.columns {
				if ts.columns[i].name == colName {
					ts.columns[i].nullable = true
				}
			}
		}
	}

	return tables, nil
}

// normalizeCR replaces bare \r (not followed by \n) with \n.
func normalizeCR(data []byte) []byte {
	var buf bytes.Buffer
	buf.Grow(len(data))
	for i := 0; i < len(data); i++ {
		if data[i] == '\r' {
			if i+1 < len(data) && data[i+1] == '\n' {
				buf.WriteByte('\r')
				buf.WriteByte('\n')
				i++ // skip the \n, we already wrote it
			} else {
				buf.WriteByte('\n')
			}
		} else {
			buf.WriteByte(data[i])
		}
	}
	return buf.Bytes()
}

// generateDDL produces CREATE TABLE and CREATE UNIQUE INDEX SQL statements from
// the parsed schemas and foreign key definitions. Unique indexes are derived
// from FK parent columns so that PRAGMA foreign_key_check can validate the data.
// Returns two slices: CREATE TABLE statements and CREATE UNIQUE INDEX statements,
// so the caller can load data between creating tables and creating indexes.
func generateDDL(tables map[string]*tableSchema, fks []foreignKey) (createTables []string, createIndexes []string) {
	// Build a lookup from child table to its foreign keys.
	fkMap := make(map[string][]foreignKey)
	for _, fk := range fks {
		fkMap[fk.childTable] = append(fkMap[fk.childTable], fk)
	}

	// Collect unique indexes needed on parent tables (deduplicated).
	type parentKey struct {
		table   string
		columns string // joined for dedup
	}
	seen := make(map[parentKey]bool)
	var uniqueIndexes []foreignKey
	for _, fk := range fks {
		pk := parentKey{fk.parentTable, strings.Join(fk.columns, ",")}
		if !seen[pk] {
			seen[pk] = true
			uniqueIndexes = append(uniqueIndexes, fk)
		}
	}

	// Sort table names for deterministic output.
	names := make([]string, 0, len(tables))
	for name := range tables {
		names = append(names, name)
	}
	sort.Strings(names)

	createTables = make([]string, 0, len(names))
	for _, name := range names {
		ts := tables[name]
		var b strings.Builder
		fmt.Fprintf(&b, "CREATE TABLE %q (\n", ts.name)

		for i, col := range ts.columns {
			fmt.Fprintf(&b, "  %q %s", col.name, col.dataType)
			if !col.nullable {
				b.WriteString(" NOT NULL")
			}
			if i < len(ts.columns)-1 || len(fkMap[name]) > 0 {
				b.WriteByte(',')
			}
			b.WriteByte('\n')
		}

		for i, fk := range fkMap[name] {
			quotedCols := make([]string, len(fk.columns))
			for j, c := range fk.columns {
				quotedCols[j] = fmt.Sprintf("%q", c)
			}
			fmt.Fprintf(&b, "  FOREIGN KEY (%s) REFERENCES %q (%s)",
				strings.Join(quotedCols, ", "),
				fk.parentTable,
				strings.Join(quotedCols, ", "),
			)
			if i < len(fkMap[name])-1 {
				b.WriteByte(',')
			}
			b.WriteByte('\n')
		}

		b.WriteString(");")
		createTables = append(createTables, b.String())
	}

	// Generate CREATE UNIQUE INDEX for each FK parent key.
	sort.Slice(uniqueIndexes, func(i, j int) bool {
		return uniqueIndexes[i].parentTable < uniqueIndexes[j].parentTable
	})
	createIndexes = make([]string, 0, len(uniqueIndexes))
	for _, fk := range uniqueIndexes {
		quotedCols := make([]string, len(fk.columns))
		for j, c := range fk.columns {
			quotedCols[j] = fmt.Sprintf("%q", c)
		}
		idxName := fmt.Sprintf("idx_%s_%s", fk.parentTable, strings.Join(fk.columns, "_"))
		stmt := fmt.Sprintf("CREATE UNIQUE INDEX %q ON %q (%s);",
			idxName, fk.parentTable, strings.Join(quotedCols, ", "))
		createIndexes = append(createIndexes, stmt)
	}

	return createTables, createIndexes
}
