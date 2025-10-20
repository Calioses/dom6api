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

type Item struct {
	Entity
	Type       string `json:"type"`
	ConstLevel int    `json:"constlevel"`
	MainLevel  int    `json:"mainlevel"`
	MPath      string `json:"mpath"`
	GemCost    string `json:"gemcost"`
}

type Merc struct {
	Entity
	BossName    string `json:"bossname"`
	CommanderID int    `json:"commander_id"`
	UnitID      int    `json:"unit_id"`
	NRUnits     int    `json:"nrunits"`
}

type Site struct {
	Entity
	Path   string            `json:"path"`
	Level  int               `json:"level"`
	Rarity string            `json:"rarity"`
	Props  map[string]string `json:"props"`
}

type Spell struct {
	Entity
	GemCost       string `json:"gemcost"`
	MPath         string `json:"mpath"`
	Type          string `json:"type"`
	School        string `json:"school"`
	ResearchLevel int    `json:"researchlevel"`
}

type Unit struct {
	Entity
	HP    int                 `json:"hp"`
	Size  int                 `json:"size"`
	Props map[string][]string `json:"props"`
}

// TODO figure out what this function was for
func NewUnit(props map[string][]string) *Unit {
	if paths, ok := props["randompaths"]; ok {
		decoded := make([]string, len(paths))
		for i, p := range paths {
			decoded[i] = p // in Go, JSON decode would be done separately if needed
		}
		props["randompaths"] = decoded
	}
	return &Unit{Props: props}
}

// TODO figure out what this function was for
func NewSite(props map[string]string) *Site {
	excluded := map[string]struct{}{
		"F":   {},
		"A":   {},
		"W":   {},
		"E":   {},
		"S":   {},
		"D":   {},
		"N":   {},
		"B":   {},
		"loc": {},
	}
	cleanProps := make(map[string]string)
	for k, v := range props {
		if _, ok := excluded[k]; !ok {
			cleanProps[k] = v
		}
	}
	return &Site{Props: cleanProps}
}

const DBFile = "Data/dom6api.db"

func handleQuery(db *sql.DB, table string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		part := strings.TrimPrefix(r.URL.Path, "/"+table+"/")
		if part == "" {
			http.Error(w, "Missing name or ID", http.StatusBadRequest)
			return
		}

		var entities []*Entity
		if id, err := strconv.Atoi(part); err == nil {
			e, err := FromID(db, table, id)
			if err == nil {
				entities = append(entities, e)
			}
		} else if isAlphabetical(part) {
			ents, _ := EntitiesByName(db, table, part)
			entities = append(entities, ents...)
		} else {
			http.Error(w, "Invalid query format", http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{table: entities})
	}
}

func FromID(db *sql.DB, table string, id int) (*Entity, error) {
	var name string
	err := db.QueryRow("SELECT name FROM "+table+" WHERE id=?", id).Scan(&name)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("not found")
		}
		return nil, err
	}

	capTable := strings.ToUpper(table[:1]) + table[1:]
	pngPath := fmt.Sprintf("/data/%s/%d.png", capTable, id)

	return &Entity{
		ID:         id,
		Name:       name,
		Table:      table,
		Screenshot: pngPath,
	}, nil
}

func EntitiesByName(db *sql.DB, table, name string) ([]*Entity, error) {
	rows, err := db.Query("SELECT id, name FROM " + table)
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
		if isAlphabetical(name) {
			score := fuzzy.Ratio(nameUpper, rowUpper)
			partial := fuzzy.PartialRatio(nameUpper, rowUpper)
			if score >= 70 || partial >= 85 {
				matches = append(matches, id)
			}
		}
	}

	var entities []*Entity
	for _, id := range matches {
		e, err := FromID(db, table, id)
		if err == nil {
			entities = append(entities, e)
		}
	}
	return entities, nil
}

func main() {
	tables := []string{"item", "spell", "unit", "site", "merc", "event"}
	db := DBcheck("dom6api.db", tables)
	defer db.Close()

	for _, table := range tables {
		http.HandleFunc("/"+table+"/", handleQuery(db, table))
	}
	log.Println("Server running on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

//------------------- Data base -----------------

func DBcheck(filename string, tables []string) *sql.DB {
	db, err := sql.Open("sqlite", filename)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}

	if _, err := os.Stat(filename); os.IsNotExist(err) {
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

	return db
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
func ScreenshotForID(table string, id int) (string, error) {
	dir := filepath.Join("Data", strings.Title(table)) // Data/Item, Data/Spell, etc.
	pngFile := filepath.Join(dir, fmt.Sprintf("%d.png", id))

	if _, err := os.Stat(pngFile); os.IsNotExist(err) {
		return "", fmt.Errorf("screenshot not found for ID %d in %s", id, table)
	}
	return pngFile, nil
}

//TODO re-add all the edgecases from the previous app
//TODO add mount edgcases. Mount, co-rider
//TODO add glamour
//TODO trim
