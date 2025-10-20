package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"

	"github.com/lithammer/fuzzysearch/fuzzy"

	_ "modernc.org/sqlite"
)

const (
	DBFile     = "Data/dom6api.db"
	FuzzyScore = 70
)

var (
	tables          = []string{"items", "spells", "units", "sites", "mercs"}
	cleanRe         = regexp.MustCompile(`[^a-zA-Z0-9 ]+`)
	tableColumns    = map[string][]string{}
	tableColumnSets = make(map[string]map[string]struct{})
)

// ------------------- API -----------------
func initDB(filename string, tables []string) (*sql.DB, error) {
	diskDB, err := sql.Open("sqlite", filename)
	if err != nil {
		return nil, err
	}
	defer diskDB.Close()

	memDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		return nil, err
	}

	for _, table := range tables {
		sqlStmt := fmt.Sprintf(
			"ATTACH DATABASE '%s' AS disk; CREATE TABLE %s AS SELECT * FROM disk.%s;",
			filename, table, table,
		)
		if _, err := memDB.Exec(sqlStmt); err != nil {
			return nil, err
		}
	}

	for _, table := range tables {
		rows, err := memDB.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
		if err != nil {
			return nil, err
		}

		cols := make([]string, 0)
		columnSet := make(map[string]struct{})

		for rows.Next() {
			var cid int
			var name, ctype string
			var notnull, dflt_value, pk any
			if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt_value, &pk); err != nil {
				continue
			}
			cols = append(cols, name)
			columnSet[name] = struct{}{}
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}

		tableColumns[table] = cols
		tableColumnSets[table] = columnSet
	}

	return memDB, nil
}

func handleQuery(db *sql.DB, table string) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		table = cleanRe.ReplaceAllString(table, "")
		columnSet, tableExists := tableColumnSets[table]
		if !tableExists {
			http.Error(w, `{"error":"unknown table"}`, http.StatusBadRequest)
			return
		}

		queryParams := request.URL.Query()
		enableFuzzyMatching := queryParams.Get("match") == "fuzzy"
		queryParams.Del("match")

		if idPart := strings.TrimPrefix(request.URL.Path, "/"+table+"/"); idPart != "" {
			if cleanID := cleanRe.ReplaceAllString(idPart, ""); cleanID != "" {
				queryParams.Set("id", cleanID)
			}
		}

		var rows *sql.Rows
		var err error

		// Optimize for ID lookups
		if ids, ok := queryParams["id"]; ok && len(ids) == 1 && !enableFuzzyMatching {
			rows, err = db.Query("SELECT * FROM "+table+" WHERE id = ?", ids[0])
		} else {
			rows, err = db.Query("SELECT * FROM " + table)
		}
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err), http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		columnNames, _ := rows.Columns()
		var results []map[string]any

		for rows.Next() {
			values := make([]any, len(columnNames))
			for i := range values {
				values[i] = new(any)
			}
			if err := rows.Scan(values...); err != nil {
				continue
			}

			row := make(map[string]any, len(columnNames))
			for i, name := range columnNames {
				val := *(values[i].(*any))
				if b, ok := val.([]byte); ok {
					row[name] = string(b)
				} else {
					row[name] = val
				}
			}

			matched := true
			for key, vals := range queryParams {
				if _, ok := columnSet[key]; !ok {
					http.Error(w, fmt.Sprintf(`{"error":"unknown column '%s'"}`, key), http.StatusBadRequest)
					return
				}

				colVal := strings.ToLower(fmt.Sprint(row[key]))
				queryVal := strings.ToLower(cleanRe.ReplaceAllString(vals[0], ""))

				if enableFuzzyMatching {
					if !strings.Contains(colVal, queryVal) && fuzzy.RankMatch(queryVal, colVal) < FuzzyScore {
						matched = false
						break
					}
				} else if colVal != queryVal {
					matched = false
					break
				}
			}

			if matched {
				id := fmt.Sprint(row["id"])
				row["image"] = fmt.Sprintf("Data/%s/%s.png", table, id)
				results = append(results, row)
			}
		}

		json.NewEncoder(w).Encode(map[string]any{table: results})
	}
}

func StartServer(dbFile string, addr string) error {
	db, err := initDB(dbFile, tables)
	if err != nil {
		return fmt.Errorf("failed to initialize DB and columns: %w", err)
	}
	defer db.Close()

	for _, table := range tables {
		http.HandleFunc("/"+table, handleQuery(db, table))
		http.HandleFunc("/"+table+"/", handleQuery(db, table))
	}

	log.Printf("Server running on %s", addr)
	return http.ListenAndServe(addr, nil)
}

func main() {
	if err := StartServer("dom6api.db", ":8080"); err != nil {
		log.Fatal(err)
	}
}

// --- Scrape ---
func dbcheck(filename string, sqlFile string) *sql.DB {
	db, err := sql.Open("sqlite", filename)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}

	if _, err := os.Stat(filename); os.IsNotExist(err) {
		sqlBytes, err := os.ReadFile(sqlFile)
		if err != nil {
			log.Fatalf("Failed to read SQL file: %v", err)
		}

		sqlStatements := string(sqlBytes)
		_, err = db.Exec(sqlStatements)
		if err != nil {
			log.Fatalf("Failed to execute SQL file: %v", err)
		}
	}

	return db
}
