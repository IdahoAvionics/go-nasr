package nasr

import (
	"archive/zip"
	"bufio"
	"database/sql"
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
	"strings"

	_ "modernc.org/sqlite"
)

func loadAllCSVs(db *sql.DB, zr *zip.Reader, tables map[string]*tableSchema) error {
	for _, f := range zr.File {
		if !strings.HasSuffix(f.Name, ".csv") {
			continue
		}
		if strings.HasSuffix(f.Name, "_CSV_DATA_STRUCTURE.csv") {
			continue
		}

		// Derive table name: take the base filename and strip .csv
		name := f.Name
		if idx := strings.LastIndex(name, "/"); idx >= 0 {
			name = name[idx+1:]
		}
		tableName := strings.TrimSuffix(name, ".csv")

		schema, ok := tables[tableName]
		if !ok {
			continue
		}

		rc, err := f.Open()
		if err != nil {
			return fmt.Errorf("open %s: %w", f.Name, err)
		}
		err = loadCSV(db, rc, schema)
		rc.Close()
		if err != nil {
			return fmt.Errorf("load %s: %w", tableName, err)
		}
	}
	return nil
}

func loadCSV(db *sql.DB, r io.Reader, schema *tableSchema) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	placeholders := make([]string, len(schema.columns))
	for i := range placeholders {
		placeholders[i] = "?"
	}
	query := fmt.Sprintf(`INSERT INTO "%s" VALUES (%s)`, schema.name, strings.Join(placeholders, ", "))

	stmt, err := tx.Prepare(query)
	if err != nil {
		return err
	}
	defer stmt.Close()

	// Strip UTF-8 BOM if present (some NASR CSVs start with \xef\xbb\xbf).
	br := bufio.NewReader(r)
	if bom, err := br.Peek(3); err == nil && len(bom) == 3 && bom[0] == 0xEF && bom[1] == 0xBB && bom[2] == 0xBF {
		br.Discard(3)
	}

	cr := csv.NewReader(br)

	// Read and discard header row.
	if _, err := cr.Read(); err != nil {
		return err
	}

	for {
		row, err := cr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		vals := make([]interface{}, len(schema.columns))
		for i, col := range schema.columns {
			if i < len(row) {
				vals[i] = convertValue(row[i], col)
			} else {
				vals[i] = nil
			}
		}

		if _, err := stmt.Exec(vals...); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func convertValue(val string, col columnDef) interface{} {
	if val == "" && col.nullable {
		return nil
	}
	if col.dataType == "REAL" && val != "" {
		if f, err := strconv.ParseFloat(val, 64); err == nil {
			return f
		}
		return val
	}
	return val
}
