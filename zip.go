package nasr

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"strings"
)

// openInnerCSVZip opens the outer NASR subscription zip and extracts the inner
// CSV zip (e.g. CSV_Data/19_Feb_2026_CSV.zip). It returns a zip.Reader over the
// inner zip and the raw bytes backing it (the caller must keep the bytes alive
// for the lifetime of the reader).
func openInnerCSVZip(outerZipPath string) (*zip.Reader, []byte, error) {
	outer, err := zip.OpenReader(outerZipPath)
	if err != nil {
		return nil, nil, fmt.Errorf("open outer zip: %w", err)
	}
	defer outer.Close()

	var innerFile *zip.File
	for _, f := range outer.File {
		// Match CSV_Data/*_CSV.zip but exclude delta zips that contain a
		// hyphen between two date strings (e.g. 19_Feb_2026-20_Mar_2026_CSV.zip).
		if !strings.HasPrefix(f.Name, "CSV_Data/") || !strings.HasSuffix(f.Name, "_CSV.zip") {
			continue
		}
		base := strings.TrimPrefix(f.Name, "CSV_Data/")
		base = strings.TrimSuffix(base, "_CSV.zip")
		if strings.Contains(base, "-") {
			continue // delta zip
		}
		innerFile = f
		break
	}
	if innerFile == nil {
		return nil, nil, fmt.Errorf("no CSV_Data/*_CSV.zip entry found in %s", outerZipPath)
	}

	rc, err := innerFile.Open()
	if err != nil {
		return nil, nil, fmt.Errorf("open inner zip entry %s: %w", innerFile.Name, err)
	}
	defer rc.Close()

	data, err := io.ReadAll(rc)
	if err != nil {
		return nil, nil, fmt.Errorf("read inner zip entry %s: %w", innerFile.Name, err)
	}

	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, nil, fmt.Errorf("open inner zip reader: %w", err)
	}

	return zr, data, nil
}
