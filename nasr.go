package nasr

import (
	"archive/zip"
	"bytes"
	"database/sql"
	"fmt"
	"log"
	"os"
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

	ddl := generateDDL(tables, foreignKeyDefs())
	for _, stmt := range ddl {
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

	rows, err := db.Query("PRAGMA foreign_key_check")
	if err != nil {
		log.Printf("foreign_key_check: %v", err)
	} else {
		defer rows.Close()
		for rows.Next() {
			var table, rowid, parent, fkid string
			if err := rows.Scan(&table, &rowid, &parent, &fkid); err != nil {
				log.Printf("foreign_key_check scan: %v", err)
				break
			}
			log.Printf("foreign key violation: table=%s rowid=%s parent=%s fkid=%s", table, rowid, parent, fkid)
		}
	}

	return nil
}
