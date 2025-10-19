package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	fuzzy "github.com/paul-mannino/go-fuzzywuzzy"
	_ "modernc.org/sqlite"
)

type Entity struct {
	ID         int    `json:"id"`
	Name       string `json:"name"`
	Table      string `json:"-"`
	Screenshot string `json:"screenshot"`
}

const DBFile = "Data/dom6api.db"

func ScreenshotForID(table string, id int) (string, error) {
	dir := filepath.Join("Data", strings.Title(table)) // Data/Item, Data/Spell, etc.
	pngFile := filepath.Join(dir, fmt.Sprintf("%d.png", id))

	if _, err := os.Stat(pngFile); os.IsNotExist(err) {
		return "", fmt.Errorf("screenshot not found for ID %d in %s", id, table)
	}
	return pngFile, nil
}

func FromID(table string, id int) (*Entity, error) {
	db, err := sql.Open("sqlite", DBFile)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	var name string
	err = db.QueryRow("SELECT name FROM "+table+" WHERE id=?", id).Scan(&name)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("not found")
		}
		return nil, err
	}

	// Capitalize first letter of table name for folder
	capTable := strings.ToUpper(table[:1]) + table[1:]

	pngPath := fmt.Sprintf("/data/%s/%d.png", capTable, id)

	return &Entity{
		ID:         id,
		Name:       name,
		Table:      table,
		Screenshot: pngPath,
	}, nil
}

func handleGetByID(table string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		idStr := strings.TrimPrefix(r.URL.Path, "/"+table+"/")
		id, err := strconv.Atoi(idStr)
		if err != nil {
			http.Error(w, "Invalid ID", http.StatusBadRequest)
			return
		}
		entity, err := FromID(table, id)
		if err != nil {
			http.Error(w, "Not Found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(entity)
	}
}

func EntitiesByName(table, name string, extraFilters map[string]string) ([]*Entity, error) {
	rows, err := MemoryDB.Query("SELECT id, name FROM " + table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	nameUpper := strings.ToUpper(name)
	var matches []int
	for rows.Next() {
		var id int
		var rowName string
		if err := rows.Scan(&id, &rowName); err != nil {
			continue
		}
		rowUpper := strings.ToUpper(rowName)

		// Only fuzzy match alphabetical queries
		if isAlphabetical(name) {
			score := fuzzy.Ratio(nameUpper, rowUpper)
			partial := fuzzy.PartialRatio(nameUpper, rowUpper)
			if score >= 70 || partial >= 85 {
				matches = append(matches, id)
			}
		} else {
			// Exact match for numeric or non-alphabetic
			if nameUpper == rowUpper {
				matches = append(matches, id)
			}
		}
	}

	var entities []*Entity
	for _, id := range matches {
		e, err := FromID(table, id)
		if err == nil {
			entities = append(entities, e)
		}
	}
	return entities, nil
}

func handleGetByName(table string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("name")
		if q == "" {
			http.Error(w, "Query parameter 'name' required", http.StatusBadRequest)
			return
		}
		filters := map[string]string{}
		if table == "unit" {
			if size := r.URL.Query().Get("size"); size != "" {
				filters["size"] = size
			}
		}
		entities, _ := EntitiesByName(table, q, filters)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{table: entities})
	}
}

func main() {
	tables := []string{"item", "spell", "unit", "site", "merc", "event"}
	DBcheck("dom6api.db", tables)
	for _, table := range tables {
		http.HandleFunc("/"+table+"/", handleGetByID(table))
		http.HandleFunc("/"+table, handleGetByName(table))
	}
	log.Println("Server running on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

//------------------- Data base -----------------

var MemoryDB *sql.DB

func DBcheck(filename string, tables []string) {
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		db, err := sql.Open("sqlite", filename)
		if err != nil {
			log.Fatalf("Failed to create database: %v", err)
		}
		defer db.Close()

		for _, table := range tables {
			_, err := db.Exec(fmt.Sprintf(`
				CREATE TABLE IF NOT EXISTS %s (
					id INTEGER PRIMARY KEY,
					name TEXT
				);
			`, table))
			if err != nil {
				log.Fatalf("Failed to create table %s: %v", table, err)
			}
		}
	}
}

func LoadInMemoryDBFromFile(filename string, tables []string) {
	memDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		log.Fatalf("Failed to open in-memory DB: %v", err)
	}
	MemoryDB = memDB

	diskDB, err := sql.Open("sqlite", filename)
	if err != nil {
		log.Fatalf("Failed to open disk DB: %v", err)
	}
	defer diskDB.Close()

	for _, table := range tables {
		_, err := memDB.Exec(fmt.Sprintf(`
			CREATE TABLE %s (
				id INTEGER PRIMARY KEY,
				name TEXT
			)
		`, table))
		if err != nil {
			log.Fatalf("Failed to create table %s in memory: %v", table, err)
		}

		rows, err := diskDB.Query("SELECT id, name FROM " + table)
		if err != nil {
			log.Fatalf("Failed to read table %s from disk: %v", table, err)
		}

		tx, _ := memDB.Begin()
		stmt, _ := tx.Prepare(fmt.Sprintf("INSERT INTO %s (id, name) VALUES (?, ?)", table))

		for rows.Next() {
			var id int
			var name string
			rows.Scan(&id, &name)
			stmt.Exec(id, name)
		}
		tx.Commit()
		rows.Close()
	}
}

// ------------------------- Helpers ----------------------------------------
func isAlphabetical(s string) bool {
	for _, r := range s {
		if r < 'A' || r > 'Z' {
			return false
		}
	}
	return true
}
