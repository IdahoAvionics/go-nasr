package nasr

import (
	"archive/zip"
	"bytes"
	"database/sql"
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"
)

// Extract reads a NASR 28-day subscription zip file and writes its CSV data
// into a new SQLite database at the given path. The output database must not
// already exist.
func Extract(nasrSubscription, sqliteDatabase string) error {
	if _, err := os.Stat(nasrSubscription); err != nil {
		return fmt.Errorf("input file: %w", err)
	}

	if _, err := os.Stat(sqliteDatabase); err == nil {
		return fmt.Errorf("output file already exists: %s", sqliteDatabase)
	}

	innerZip, data, err := openInnerCSVZip(nasrSubscription)
	if err != nil {
		return fmt.Errorf("open inner CSV zip: %w", err)
	}

	tables, err := parseSchemas(innerZip)
	if err != nil {
		return fmt.Errorf("parse schemas: %w", err)
	}

	db, err := sql.Open("sqlite", sqliteDatabase)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	if _, err := db.Exec("PRAGMA journal_mode = WAL"); err != nil {
		return fmt.Errorf("set journal_mode: %w", err)
	}
	if _, err := db.Exec("PRAGMA synchronous = OFF"); err != nil {
		return fmt.Errorf("set synchronous: %w", err)
	}

	fks := foreignKeyDefs()
	createTables, createIndexes := generateDDL(tables, fks)
	for _, stmt := range createTables {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("create table: %w\n%s", err, stmt)
		}
	}

	// Re-create a fresh zip.Reader since parseSchemas consumed the first one.
	innerZip, err = zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return fmt.Errorf("reopen inner zip: %w", err)
	}

	if err := loadAllCSVs(db, innerZip, tables); err != nil {
		return fmt.Errorf("load CSVs: %w", err)
	}

	// Deduplicate parent tables and create unique indexes. If an index fails
	// due to duplicate source data, the duplicates are deleted (keeping the
	// lowest rowid) and the index is retried.
	if err := deduplicateParents(db, createIndexes); err != nil {
		return fmt.Errorf("deduplicate parents: %w", err)
	}

	// Delete FK orphan rows — child rows referencing non-existent parents.
	if err := deleteOrphans(db); err != nil {
		return fmt.Errorf("delete orphans: %w", err)
	}

	// Final FK check — hard failure if any violations remain.
	rows, err := db.Query("PRAGMA foreign_key_check")
	if err != nil {
		return fmt.Errorf("final foreign_key_check: %w", err)
	}
	defer rows.Close()
	var fkViolations int
	for rows.Next() {
		var table, rowid, parent, fkid string
		if err := rows.Scan(&table, &rowid, &parent, &fkid); err != nil {
			return fmt.Errorf("scan foreign_key_check: %w", err)
		}
		log.Printf("FK violation remaining: table=%s rowid=%s parent=%s fkid=%s", table, rowid, parent, fkid)
		fkViolations++
	}
	if fkViolations > 0 {
		return fmt.Errorf("foreign key check failed: %d violations remain", fkViolations)
	}

	return nil
}

// deduplicateParents attempts to create each unique index. If creation fails
// due to duplicate rows, it deletes duplicates (keeping the lowest rowid),
// logs a warning, and retries. Returns an error if any index cannot be created.
func deduplicateParents(db *sql.DB, createIndexes []string) error {
	for _, stmt := range createIndexes {
		_, err := db.Exec(stmt)
		if err == nil {
			continue
		}

		// Index creation failed. Parse statement to find table/columns.
		table, columns, parseErr := parseUniqueIndex(stmt)
		if parseErr != nil {
			return fmt.Errorf("create index: %w\n%s", err, stmt)
		}

		// Build quoted column list.
		quotedCols := make([]string, len(columns))
		for i, c := range columns {
			quotedCols[i] = fmt.Sprintf("%q", c)
		}
		colList := strings.Join(quotedCols, ", ")

		// Find groups with duplicate keys.
		query := fmt.Sprintf(
			"SELECT %s FROM %q GROUP BY %s HAVING count(*) > 1",
			colList, table, colList,
		)
		dupRows, err := db.Query(query)
		if err != nil {
			return fmt.Errorf("find duplicates in %s: %w", table, err)
		}

		for dupRows.Next() {
			vals := make([]interface{}, len(columns))
			ptrs := make([]interface{}, len(columns))
			for i := range vals {
				ptrs[i] = &vals[i]
			}
			if err := dupRows.Scan(ptrs...); err != nil {
				dupRows.Close()
				return fmt.Errorf("scan duplicate key in %s: %w", table, err)
			}

			// Build WHERE clause.
			whereParts := make([]string, len(columns))
			whereVals := make([]interface{}, len(columns))
			for i, col := range columns {
				whereParts[i] = fmt.Sprintf("%q = ?", col)
				whereVals[i] = vals[i]
			}
			whereClause := strings.Join(whereParts, " AND ")

			// Find all rowids for this key.
			rowidQuery := fmt.Sprintf("SELECT rowid FROM %q WHERE %s ORDER BY rowid",
				table, whereClause)
			rowidRows, err := db.Query(rowidQuery, whereVals...)
			if err != nil {
				dupRows.Close()
				return fmt.Errorf("find rowids in %s: %w", table, err)
			}

			var rowids []int64
			for rowidRows.Next() {
				var rid int64
				if err := rowidRows.Scan(&rid); err != nil {
					rowidRows.Close()
					dupRows.Close()
					return fmt.Errorf("scan rowid in %s: %w", table, err)
				}
				rowids = append(rowids, rid)
			}
			rowidRows.Close()

			if len(rowids) < 2 {
				continue
			}

			// Delete all but the lowest rowid.
			for _, rid := range rowids[1:] {
				keyParts := make([]string, len(columns))
				for i, col := range columns {
					keyParts[i] = fmt.Sprintf("%s=%v", col, vals[i])
				}
				log.Printf("WARNING: deleted duplicate row from %s (kept rowid %d, deleted rowid %d, key: %s)",
					table, rowids[0], rid, strings.Join(keyParts, ", "))
				if _, err := db.Exec(fmt.Sprintf("DELETE FROM %q WHERE rowid = ?", table), rid); err != nil {
					dupRows.Close()
					return fmt.Errorf("delete duplicate rowid %d from %s: %w", rid, table, err)
				}
			}
		}
		dupRows.Close()

		// Retry index creation.
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("create index after dedup: %w\n%s", err, stmt)
		}
	}
	return nil
}

var uniqueIndexRe = regexp.MustCompile(`ON\s+"([^"]+)"\s+\(([^)]+)\)`)

// parseUniqueIndex extracts the table name and column names from a
// CREATE UNIQUE INDEX statement.
func parseUniqueIndex(stmt string) (string, []string, error) {
	m := uniqueIndexRe.FindStringSubmatch(stmt)
	if m == nil {
		return "", nil, fmt.Errorf("cannot parse index statement: %s", stmt)
	}
	table := m[1]
	var columns []string
	for _, col := range strings.Split(m[2], ",") {
		col = strings.TrimSpace(col)
		col = strings.Trim(col, `"`)
		columns = append(columns, col)
	}
	return table, columns, nil
}

// deleteOrphans runs PRAGMA foreign_key_check and deletes any child rows
// that reference non-existent parent rows. Logs a warning for each deletion.
func deleteOrphans(db *sql.DB) error {
	rows, err := db.Query("PRAGMA foreign_key_check")
	if err != nil {
		return fmt.Errorf("foreign_key_check: %w", err)
	}
	defer rows.Close()

	type violation struct {
		table  string
		rowid  string
		parent string
		fkid   string
	}

	var violations []violation
	for rows.Next() {
		var v violation
		if err := rows.Scan(&v.table, &v.rowid, &v.parent, &v.fkid); err != nil {
			return fmt.Errorf("scan foreign_key_check: %w", err)
		}
		violations = append(violations, v)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("foreign_key_check iteration: %w", err)
	}

	for _, v := range violations {
		log.Printf("WARNING: deleted orphan row from %s (rowid %s, missing parent in %s)",
			v.table, v.rowid, v.parent)
		if _, err := db.Exec(fmt.Sprintf("DELETE FROM %q WHERE rowid = ?", v.table), v.rowid); err != nil {
			return fmt.Errorf("delete orphan rowid %s from %s: %w", v.rowid, v.table, err)
		}
	}

	return nil
}
