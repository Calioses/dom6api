package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strings"

	"github.com/lithammer/fuzzysearch/fuzzy"

	_ "modernc.org/sqlite"
)

const (
	FuzzyScore = 70
)

var (
	tables          = []string{"items", "spells", "units", "sites", "mercs", "events"}
	cleanRe         = regexp.MustCompile(`[^a-zA-Z0-9 ]+`)
	tableColumns    = map[string][]string{}
	tableColumnSets = make(map[string]map[string]struct{})
)

// ------------------- API -----------------
func initDB(filename string, tables []string) (*sql.DB, error) {
	log.Println("Initializing in-memory DB from file:", filename)

	memDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		return nil, err
	}

	// Attach disk database once
	if _, err := memDB.Exec(fmt.Sprintf("ATTACH DATABASE '%s' AS disk;", filename)); err != nil {
		return nil, fmt.Errorf("failed to attach disk DB: %w", err)
	}

	for _, table := range tables {
		log.Printf("Copying table '%s' into memory...\n", table)
		sqlStmt := fmt.Sprintf("CREATE TABLE %s AS SELECT * FROM disk.%s;", table, table)
		if _, err := memDB.Exec(sqlStmt); err != nil {
			return nil, fmt.Errorf("failed to copy table %s: %w", table, err)
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
		log.Printf("Table '%s' has columns: %v\n", table, cols)
	}

	log.Println("In-memory DB initialization complete.")
	return memDB, nil
}

func handleQuery(db *sql.DB, table string) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		table = cleanRe.ReplaceAllString(table, "")
		columnSet, ok := tableColumnSets[table]
		if !ok {
			http.Error(w, `{"error":"unknown table"}`, http.StatusBadRequest)
			return
		}

		queryParams := request.URL.Query()
		enableFuzzy := queryParams.Get("match") == "fuzzy"
		queryParams.Del("match")

		pathSuffix := strings.TrimPrefix(request.URL.Path, "/"+table+"/")
		if pathSuffix != "" {
			cleanID := cleanRe.ReplaceAllString(pathSuffix, "")
			if cleanID != "" {
				queryParams.Set("id", cleanID)
			}
		}

		rows, err := db.Query("SELECT * FROM " + table)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err), http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		cols, _ := rows.Columns()
		var results []map[string]any

		for rows.Next() {
			values := make([]any, len(cols))
			for i := range values {
				values[i] = new(any)
			}
			if err := rows.Scan(values...); err != nil {
				continue
			}

			row := make(map[string]any, len(cols))
			for i, name := range cols {
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

				queryVal := cleanRe.ReplaceAllString(vals[0], "")
				colVal := row[key]

				switch v := colVal.(type) {
				case int64, int32, int:
					q, err := fmt.Sscan(queryVal)
					if err != nil || fmt.Sprint(v) != fmt.Sprint(q) {
						matched = false
						break
					}
				case float64, float32:
					q, err := fmt.Sscan(queryVal)
					if err != nil || fmt.Sprint(v) != fmt.Sprint(q) {
						matched = false
						break
					}
				case string:
					cv := strings.ToLower(v)
					qv := strings.ToLower(queryVal)
					if enableFuzzy {
						if !strings.Contains(cv, qv) && fuzzy.RankMatch(qv, cv) < FuzzyScore {
							matched = false
							break
						}
					} else if cv != qv {
						matched = false
						break
					}
				default:
					if fmt.Sprint(v) != queryVal {
						matched = false
						break
					}
				}
			}

			if matched {
				row["image"] = fmt.Sprintf("Data/%s/%v.png", table, row["id"])
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

	for _, table := range tables {
		http.HandleFunc("/"+table, handleQuery(db, table))
		http.HandleFunc("/"+table+"/", handleQuery(db, table))
	}

	return http.ListenAndServe(addr, nil)
}

// func main() {
// 	if err := StartServer("dom6api.db", ":8080"); err != nil {
// 		log.Fatal(err)
// 	}
// }

// --- Scrape ---
