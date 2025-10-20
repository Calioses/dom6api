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
	DBFile = "Data/dom6api.db"

	ItemTable  = "items"
	MercTable  = "mercs"
	SpellTable = "spells"
	UnitTable  = "units"
	SiteTable  = "sites"

	FuzzyScore = 70
)

var (
	cleanRe      = regexp.MustCompile(`[^a-zA-Z0-9 ]+`)
	tableColumns = map[string][]string{}
)

type (
	Entity struct {
		ID         int    `json:"id"`
		Name       string `json:"name"`
		Table      string `json:"-"`
		Screenshot string `json:"screenshot"`
	}
)

func initColumns(db *sql.DB, tables []string) error {
	for _, table := range tables {
		rows, err := db.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
		if err != nil {
			return err
		}
		defer rows.Close()

		var cols []string
		for rows.Next() {
			var cid int
			var name, ctype string
			var notnull, dflt_value, pk any
			if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt_value, &pk); err != nil {
				continue
			}
			cols = append(cols, name)
		}
		tableColumns[table] = cols
	}
	return nil
}

func handleQuery(db *sql.DB, table string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		cols, ok := tableColumns[table]
		if !ok {
			http.Error(w, `{"error":"unknown table"}`, http.StatusBadRequest)
			return
		}
		colMap := make(map[string]struct{}, len(cols))
		for _, c := range cols {
			colMap[c] = struct{}{}
		}

		params := r.URL.Query()
		fuzzyEnabled := false
		if vals, ok := params["match"]; ok && len(vals) > 0 && vals[0] == "fuzzy" {
			fuzzyEnabled = true
			delete(params, "match")
		}

		idPart := strings.TrimPrefix(r.URL.Path, "/"+table+"/")
		if idPart != "" {
			clean := cleanRe.ReplaceAllString(idPart, "")
			if clean != "" {
				params["id"] = []string{clean}
			}
		}

		rows, err := db.Query("SELECT * FROM " + table)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err), http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		colNames, _ := rows.Columns()
		type scoredRow struct {
			score int
			id    int
			row   map[string]any
		}
		var best *scoredRow

		for rows.Next() {
			values := make([]any, len(colNames))
			ptrs := make([]any, len(colNames))
			for i := range values {
				ptrs[i] = &values[i]
			}
			if err := rows.Scan(ptrs...); err != nil {
				continue
			}

			rowMap := map[string]any{}
			for i, col := range colNames {
				rowMap[col] = values[i]
			}

			match := true
			score := 0
			for key, val := range params {
				if _, ok := colMap[key]; !ok {
					http.Error(w, fmt.Sprintf(`{"error":"unknown column '%s'"}`, key), http.StatusBadRequest)
					return
				}
				strVal := fmt.Sprintf("%v", rowMap[key])
				queryVal := val[0]

				if fuzzyEnabled {
					partial := strings.Contains(strings.ToLower(strVal), strings.ToLower(queryVal))
					fuzzyScore := fuzzy.RankMatch(queryVal, strVal)

					if partial {
						score += len(queryVal)
					} else if fuzzyScore >= FuzzyScore {
						score += fuzzyScore
					} else {
						match = false
						break
					}
				} else if strVal != queryVal {
					match = false
					break
				}
			}

			if match {
				id := 0
				if v, ok := rowMap["id"].(int); ok {
					id = v
				} else if v, ok := rowMap["id"].(int64); ok {
					id = int(v)
				}
				sr := scoredRow{score: score, id: id, row: rowMap}
				if best == nil || sr.score > best.score || (sr.score == best.score && sr.id < best.id) {
					best = &sr
				}
			}
		}

		results := []map[string]any{}
		if best != nil {
			results = append(results, best.row)
		}

		pluralTable := table
		if !strings.HasSuffix(pluralTable, "s") {
			pluralTable += "s"
		}

		json.NewEncoder(w).Encode(map[string]any{pluralTable: results})
	}
}

func main() {
	tables := []string{"items", "spells", "units", "sites", "mercs"}
	db := DBcheck("dom6api.db", "create_tables.sql")
	defer db.Close()

	if err := initColumns(db, tables); err != nil {
		log.Fatalf("failed to initialize table columns: %s", err)
	}

	for _, table := range tables {
		http.HandleFunc("/"+table+"/", handleQuery(db, table))
	}
	log.Println("Server running on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

//------------------- Data base -----------------

func DBcheck(filename string, sqlFile string) *sql.DB {
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

// ------------------------- Helpers ----------------------------------------

// TODO re-add all the edgecases from the previous app
// TODO add mount edgcases. Mount, co-rider
// TODO add glamour
// TODO trim
// TODO figure out what this function was for

/*
Got it — you want the Go server to support both:

✅ /items/{id} style routes (fetch by ID)
✅ /items?name=...&match=fuzzy style routes (query by name, with optional fuzzy matching and filters like size for units)

Before proceeding — confirm:

You already have a shared handler like handleQuery(table string) that dispatches to DB reads.

You want to keep that structure but extend it to handle these PHP-style query parameters.

Fuzzy match should mean LIKE '%name%' (SQL equivalent), correct?

Once confirmed, I'll rewrite your Go handler layer to cover the same query behavior as the PHP routes.
*/

/*
TODO
Make the query handler match the old one and add mounts into the mix.
general clean up and efficiency over the older model.
Make an in memory sqlite instance to resolve the potentially horrendous speed issues.
Make sure output format is unified.
attached PNG to ID.
*/
