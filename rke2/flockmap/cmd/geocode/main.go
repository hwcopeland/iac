// Command geocode is the flockmap P1 connector: it loads U.S. Census TIGER/Line
// geographies (States, Counties, County Subdivisions, Places) into the
// jurisdictions table, then performs a deterministic PostGIS point-in-polygon
// assignment that stamps every camera with its place_geoid / county_geoid /
// state_fips.
//
// THE FIREWALL (CONVENTIONS.md §7): no ML anywhere. Every external fetch (each
// TIGER shapefile zip) is stored verbatim via store.SaveRawDocument BEFORE it
// is parsed, and every jurisdictions row carries that raw_documents id as
// source_doc_id. The point-in-polygon assignment is pure SQL (ST_Contains over
// a GiST index) — no API in the hot path.
//
// Data flow per shapefile:
//
//	HTTP GET zip  ->  SaveRawDocument (channel census_tiger)  ->  unzip
//	   ->  ogr2ogr load into staging table tiger_raw.<table>
//	   ->  INSERT ... SELECT into jurisdictions (threading source_doc_id)
//
// After all geographies are loaded, a single set-based UPDATE per level assigns
// camera geoids via ST_Contains. ogr2ogr (GDAL) is the only external binary; it
// is provided by the container image (see deploy/geocode-job.yaml).
//
// Census Geocoder (geocoding.geo.census.gov) and the FCC Area API
// (geo.fcc.gov/api/census/area) are SPOT-CHECK tools only — useful to validate
// a handful of assignments by hand. They are deliberately NOT on the hot path:
// the authoritative assignment is the local TIGER point-in-polygon join, which
// is reproducible and provenanced.
package main

import (
	"archive/zip"
	"database/sql"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/hwcopeland/flockmap/internal/store"
)

const (
	jobName = "geocode"
	channel = "census_tiger"

	// TIGER/Line is a work of the U.S. federal government: public domain.
	tigerLicense     = "Public Domain (U.S. Census Bureau TIGER/Line)"
	tigerAttribution = "U.S. Census Bureau, TIGER/Line Shapefiles"

	// Staging schema ogr2ogr loads shapefiles into before the provenanced
	// INSERT into jurisdictions. Dropped/recreated each run.
	stagingSchema = "tiger_raw"
)

// tigerYear is the vintage of the TIGER/Line release to pull. Overridable via
// flag so a re-run can pin a vintage for reproducibility.
var tigerYear = "2023"

// fetcher downloads bytes for a URL, recording provenance.
type httpFetcher struct {
	client *http.Client
	db     *sql.DB
}

func main() {
	flag.StringVar(&tigerYear, "year", envOr("TIGER_YEAR", tigerYear), "TIGER/Line vintage year to download")
	skipLoad := flag.Bool("skip-load", false, "skip TIGER download+load, only run the point-in-polygon assignment")
	skipAssign := flag.Bool("skip-assign", false, "skip the point-in-polygon assignment, only load TIGER")
	workDir := flag.String("workdir", envOr("TIGER_WORKDIR", "/tmp/tiger"), "scratch dir for downloaded zips and extracted shapefiles")
	flag.Parse()

	log.SetFlags(log.LstdFlags | log.LUTC)
	log.Printf("geocode: starting (TIGER vintage %s)", tigerYear)

	db, err := store.ConnectPostgres()
	if err != nil {
		log.Fatalf("geocode: connect postgres: %v", err)
	}
	defer db.Close()

	if err := store.EnsureSchema(db); err != nil {
		log.Fatalf("geocode: ensure schema: %v", err)
	}

	if !*skipLoad {
		f := &httpFetcher{
			client: &http.Client{Timeout: 30 * time.Minute},
			db:     db,
		}
		if err := loadAllTiger(db, f, *workDir); err != nil {
			log.Fatalf("geocode: load TIGER: %v", err)
		}
	} else {
		log.Printf("geocode: --skip-load set, skipping TIGER download/load")
	}

	if !*skipAssign {
		if err := assignCameraGeoids(db); err != nil {
			log.Fatalf("geocode: assign camera geoids: %v", err)
		}
	} else {
		log.Printf("geocode: --skip-assign set, skipping point-in-polygon assignment")
	}

	log.Printf("geocode: done")
}

// tigerSource describes one shapefile (or set of state-partitioned shapefiles)
// to load into a single staging table, then promote into jurisdictions.
type tigerSource struct {
	level string // jurisdictions.level value

	// urls returns the list of zip URLs for this level. National-level files
	// (STATE, COUNTY) are one URL; per-state files (PLACE, COUSUB) are 56.
	urls func(year string) []string

	// table is the staging table name (tiger_raw.<table>) ogr2ogr loads into.
	table string

	// selectSQL maps staging columns into the jurisdictions INSERT. It must
	// produce columns (geoid, level, name, state_fips, geom_ewkt). $1 binds the
	// source_doc_id of the raw document the row came from.
	//
	// TIGER column names (lowercased by ogr2ogr): geoid, name, statefp, stusps.
	insertCols string
}

// stateFIPS is the full list of state/territory FIPS codes for which the Census
// publishes per-state PLACE and COUSUB shapefiles. (50 states + DC + 5 terrs.)
var stateFIPS = []string{
	"01", "02", "04", "05", "06", "08", "09", "10", "11", "12", "13", "15",
	"16", "17", "18", "19", "20", "21", "22", "23", "24", "25", "26", "27",
	"28", "29", "30", "31", "32", "33", "34", "35", "36", "37", "38", "39",
	"40", "41", "42", "44", "45", "46", "47", "48", "49", "50", "51", "53",
	"54", "55", "56", "60", "66", "69", "72", "78",
}

func tigerBase(year string) string {
	return fmt.Sprintf("https://www2.census.gov/geo/tiger/TIGER%s", year)
}

func tigerSources() []tigerSource {
	return []tigerSource{
		{
			level: "state",
			table: "tl_state",
			urls: func(y string) []string {
				return []string{fmt.Sprintf("%s/STATE/tl_%s_us_state.zip", tigerBase(y), y)}
			},
			// STATE has no statefp distinct from geoid; geoid IS the 2-char FIPS.
			insertCols: `geoid AS geoid, 'state' AS level, name AS name,
			             geoid AS state_fips, ST_AsEWKT(ST_SetSRID(wkb_geometry,4326)) AS geom_ewkt`,
		},
		{
			level: "county",
			table: "tl_county",
			urls: func(y string) []string {
				return []string{fmt.Sprintf("%s/COUNTY/tl_%s_us_county.zip", tigerBase(y), y)}
			},
			insertCols: `geoid AS geoid, 'county' AS level, name AS name,
			             statefp AS state_fips, ST_AsEWKT(ST_SetSRID(wkb_geometry,4326)) AS geom_ewkt`,
		},
		{
			level: "county_subdivision",
			table: "tl_cousub",
			urls: func(y string) []string {
				out := make([]string, 0, len(stateFIPS))
				for _, fp := range stateFIPS {
					out = append(out, fmt.Sprintf("%s/COUSUB/tl_%s_%s_cousub.zip", tigerBase(y), y, fp))
				}
				return out
			},
			insertCols: `geoid AS geoid, 'county_subdivision' AS level, name AS name,
			             statefp AS state_fips, ST_AsEWKT(ST_SetSRID(wkb_geometry,4326)) AS geom_ewkt`,
		},
		{
			level: "place",
			table: "tl_place",
			urls: func(y string) []string {
				out := make([]string, 0, len(stateFIPS))
				for _, fp := range stateFIPS {
					out = append(out, fmt.Sprintf("%s/PLACE/tl_%s_%s_place.zip", tigerBase(y), y, fp))
				}
				return out
			},
			insertCols: `geoid AS geoid, 'place' AS level, name AS name,
			             statefp AS state_fips, ST_AsEWKT(ST_SetSRID(wkb_geometry,4326)) AS geom_ewkt`,
		},
	}
}

func loadAllTiger(db *sql.DB, f *httpFetcher, workDir string) error {
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		return fmt.Errorf("mkdir workdir: %w", err)
	}
	if _, err := db.Exec(fmt.Sprintf("CREATE SCHEMA IF NOT EXISTS %s", stagingSchema)); err != nil {
		return fmt.Errorf("create staging schema: %w", err)
	}

	for _, src := range tigerSources() {
		if err := loadSource(db, f, workDir, src); err != nil {
			return fmt.Errorf("level %s: %w", src.level, err)
		}
	}
	return nil
}

// loadSource downloads every zip for one level, stores each verbatim
// (provenance), loads it into the staging table via ogr2ogr, then INSERTs the
// staged rows into jurisdictions threading the per-file source_doc_id.
func loadSource(db *sql.DB, f *httpFetcher, workDir string, src tigerSource) error {
	staging := fmt.Sprintf("%s.%s", stagingSchema, src.table)

	urls := src.urls(tigerYear)
	log.Printf("geocode: level=%s loading %d shapefile(s) into %s", src.level, len(urls), staging)

	// Fresh staging table each run: append mode loads all per-state files into
	// the same table, so drop first.
	if _, err := db.Exec(fmt.Sprintf("DROP TABLE IF EXISTS %s", staging)); err != nil {
		return fmt.Errorf("drop staging %s: %w", staging, err)
	}

	for i, url := range urls {
		zipPath := filepath.Join(workDir, filepath.Base(url))

		raw, err := f.fetch(url)
		if err != nil {
			return fmt.Errorf("fetch %s: %w", url, err)
		}

		// Store verbatim BEFORE parsing — firewall requirement.
		docID, err := store.SaveRawDocument(db, store.RawDocument{
			SourceURL:    url,
			HTTPMethod:   "GET",
			ContentType:  "application/zip",
			RawBytes:     raw,
			Channel:      channel,
			FetchedByJob: jobName,
			License:      tigerLicense,
			Attribution:  tigerAttribution,
		})
		if err != nil {
			return fmt.Errorf("save raw doc %s: %w", url, err)
		}

		if err := os.WriteFile(zipPath, raw, 0o644); err != nil {
			return fmt.Errorf("write zip %s: %w", zipPath, err)
		}
		shpPath, cleanup, err := extractShp(zipPath, workDir)
		if err != nil {
			return fmt.Errorf("unzip %s: %w", zipPath, err)
		}

		// First file creates the table; subsequent files append.
		appendMode := i > 0
		if err := ogr2ogrLoad(shpPath, staging, appendMode); err != nil {
			cleanup()
			return fmt.Errorf("ogr2ogr %s: %w", shpPath, err)
		}

		// Promote this file's rows into jurisdictions with its provenance id.
		// We tag staged rows with the source doc id so each file's rows are
		// attributed to its own raw_documents row, then promote row-by-row so
		// every PK is a real store.NewUUIDv7.
		if err := tagAndPromote(db, staging, src, docID, appendMode); err != nil {
			cleanup()
			return fmt.Errorf("promote %s: %w", staging, err)
		}

		cleanup()
		_ = os.Remove(zipPath)
		log.Printf("geocode: level=%s loaded %s (doc %s)", src.level, filepath.Base(url), docID)
	}
	return nil
}

// tagAndPromote stamps the rows ogr2ogr just appended (those whose tagging
// column is still NULL) with this file's source_doc_id, then promotes them into
// jurisdictions. Because per-state PLACE/COUSUB files all append into one
// staging table, the NULL-stamp trick keeps each file's rows attributed to its
// own raw_documents row exactly.
func tagAndPromote(db *sql.DB, staging string, src tigerSource, docID string, appendMode bool) error {
	if !appendMode {
		if _, err := db.Exec(fmt.Sprintf(
			"ALTER TABLE %s ADD COLUMN IF NOT EXISTS flockmap_src_doc UUID", staging)); err != nil {
			return fmt.Errorf("add src_doc col: %w", err)
		}
	}
	if _, err := db.Exec(fmt.Sprintf(
		"UPDATE %s SET flockmap_src_doc = $1 WHERE flockmap_src_doc IS NULL", staging), docID); err != nil {
		return fmt.Errorf("stamp src_doc: %w", err)
	}
	return promoteRowByRow(db, staging, src, docID)
}

// promoteRowByRow reads the staged rows for one file and upserts each via
// UpsertJurisdiction so every PK is a store.NewUUIDv7 and every row carries the
// file's source_doc_id. TIGER files are a few thousand rows each — well within
// a single transaction's reach.
func promoteRowByRow(db *sql.DB, staging string, src tigerSource, docID string) error {
	q := fmt.Sprintf(`SELECT %s FROM %s WHERE flockmap_src_doc = $1`, src.insertCols, staging)
	rows, err := db.Query(q, docID)
	if err != nil {
		return fmt.Errorf("select staged: %w", err)
	}
	defer rows.Close()

	var n int
	for rows.Next() {
		var j Jurisdiction
		var stateFIPS sql.NullString
		if err := rows.Scan(&j.GeoID, &j.Level, &j.Name, &stateFIPS, &j.GeomEWKT); err != nil {
			return fmt.Errorf("scan staged: %w", err)
		}
		if stateFIPS.Valid {
			j.StateFIPS = stateFIPS.String
		}
		if _, err := UpsertJurisdiction(db, j, docID); err != nil {
			return err
		}
		n++
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate staged: %w", err)
	}
	log.Printf("geocode: level=%s promoted %d rows (doc %s)", src.level, n, docID)
	return nil
}

// assignCameraGeoids runs the deterministic point-in-polygon assignment. Three
// set-based UPDATEs (one per level), each using ST_Contains over the GiST index
// on jurisdictions.geom and cameras.geom. No external API. Idempotent.
func assignCameraGeoids(db *sql.DB) error {
	log.Printf("geocode: assigning camera geoids via ST_Contains point-in-polygon")

	// state_fips first (broadest, always present for a valid US camera), then
	// county, then place. Place membership is the narrowest; a camera may fall
	// in no incorporated place (unincorporated area) -> place_geoid stays NULL.
	steps := []struct {
		name string
		sql  string
	}{
		{
			"state_fips",
			`UPDATE cameras c
			    SET state_fips = j.state_fips
			   FROM jurisdictions j
			  WHERE j.level = 'state'
			    AND ST_Contains(j.geom, c.geom)`,
		},
		{
			"county_geoid",
			`UPDATE cameras c
			    SET county_geoid = j.geoid
			   FROM jurisdictions j
			  WHERE j.level = 'county'
			    AND ST_Contains(j.geom, c.geom)`,
		},
		{
			"place_geoid",
			`UPDATE cameras c
			    SET place_geoid = j.geoid
			   FROM jurisdictions j
			  WHERE j.level = 'place'
			    AND ST_Contains(j.geom, c.geom)`,
		},
	}

	for _, s := range steps {
		res, err := db.Exec(s.sql)
		if err != nil {
			return fmt.Errorf("assign %s: %w", s.name, err)
		}
		n, _ := res.RowsAffected()
		log.Printf("geocode: assigned %s for %d cameras", s.name, n)
	}
	return nil
}

// --- HTTP fetch (provenance happens in the caller) ---

func (f *httpFetcher) fetch(url string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "flockmap-geocode/1.0 (+https://github.com/hwcopeland/flockmap)")

	resp, err := f.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return nil, fmt.Errorf("GET %s: status %d: %s", url, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return io.ReadAll(resp.Body)
}

// --- shapefile extraction + ogr2ogr load ---

// extractShp unzips a TIGER zip into a per-zip subdir and returns the path to
// the .shp. cleanup removes the extracted files.
func extractShp(zipPath, workDir string) (shp string, cleanup func(), err error) {
	base := strings.TrimSuffix(filepath.Base(zipPath), ".zip")
	dest := filepath.Join(workDir, base)
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return "", func() {}, err
	}
	cleanup = func() { _ = os.RemoveAll(dest) }

	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return "", cleanup, err
	}
	defer r.Close()

	for _, zf := range r.File {
		// Guard against zip-slip.
		name := filepath.Clean(zf.Name)
		if strings.HasPrefix(name, "..") || filepath.IsAbs(name) {
			return "", cleanup, fmt.Errorf("unsafe zip entry %q", zf.Name)
		}
		outPath := filepath.Join(dest, filepath.Base(name))
		if zf.FileInfo().IsDir() {
			continue
		}
		rc, err := zf.Open()
		if err != nil {
			return "", cleanup, err
		}
		out, err := os.Create(outPath)
		if err != nil {
			rc.Close()
			return "", cleanup, err
		}
		if _, err := io.Copy(out, rc); err != nil {
			out.Close()
			rc.Close()
			return "", cleanup, err
		}
		out.Close()
		rc.Close()
		if strings.EqualFold(filepath.Ext(outPath), ".shp") {
			shp = outPath
		}
	}
	if shp == "" {
		return "", cleanup, fmt.Errorf("no .shp found in %s", zipPath)
	}
	return shp, cleanup, nil
}

// ogr2ogrLoad loads a shapefile into a Postgres staging table via the ogr2ogr
// binary (GDAL). Connection comes from PG* env vars derived from POSTGRES_*.
// The geometry column is named wkb_geometry (ogr2ogr default for PG).
func ogr2ogrLoad(shpPath, staging string, appendMode bool) error {
	pgDSN := fmt.Sprintf("PG:host=%s port=%s dbname=%s user=%s password=%s",
		envOr("POSTGRES_HOST", "localhost"),
		envOr("POSTGRES_PORT", "5432"),
		envOr("POSTGRES_DB", "flockmap"),
		os.Getenv("POSTGRES_USER"),
		os.Getenv("POSTGRES_PASSWORD"),
	)

	args := []string{
		"-f", "PostgreSQL", pgDSN, shpPath,
		"-nln", staging, // target table (schema.table)
		"-lco", "GEOMETRY_NAME=wkb_geometry",
		"-lco", "FID=ogc_fid",
		"-lco", "PRECISION=NO",
		"-nlt", "MULTIPOLYGON",
		"-t_srs", "EPSG:4326",
		"-lco", "SPATIAL_INDEX=NONE", // staging is transient; skip index build
		"--config", "PG_USE_COPY", "YES",
	}
	if appendMode {
		args = append(args, "-append")
	} else {
		args = append(args, "-overwrite")
	}

	cmd := exec.Command("ogr2ogr", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ogr2ogr load %s -> %s: %w", filepath.Base(shpPath), staging, err)
	}
	return nil
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
