package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"

	fuzzy "github.com/paul-mannino/go-fuzzywuzzy"
	_ "modernc.org/sqlite"
)

const DBFile = "Data/dom6api.db"

type (
	Entity struct {
		ID         int    `json:"id"`
		Name       string `json:"name"`
		Table      string `json:"-"`
		Screenshot string `json:"screenshot"`
	}

	Item struct {
		Entity
		Type       string `json:"type"`
		ConstLevel int    `json:"constlevel"`
		MainLevel  int    `json:"mainlevel"`
		MPath      string `json:"mpath"`
		GemCost    string `json:"gemcost"`
	}

	Merc struct {
		Entity
		BossName    string `json:"bossname"`
		CommanderID int    `json:"commander_id"`
		UnitID      int    `json:"unit_id"`
		NRUnits     int    `json:"nrunits"`
	}

	Site struct {
		Entity
		Path   string            `json:"path"`
		Level  int               `json:"level"`
		Rarity string            `json:"rarity"`
		Props  map[string]string `json:"props"`
	}

	Spell struct {
		Entity
		GemCost       string `json:"gemcost"`
		MPath         string `json:"mpath"`
		Type          string `json:"type"`
		School        string `json:"school"`
		ResearchLevel int    `json:"researchlevel"`
	}

	Unit struct {
		Entity
		HP    int                 `json:"hp"`
		Size  int                 `json:"size"`
		Props map[string][]string `json:"props"`
	}
)

/*
TODO
Make the query handler match the old one and add mounts into the mix.
general clean up and efficiency over the older model.
Make an in memory sqlite instance to resolve the potentially horrendous speed issues.
Make sure output format is unified.
attached PNG to ID.
*/
func handleQuery(db *sql.DB, table string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		part := strings.TrimPrefix(r.URL.Path, "/"+table+"/")
		if part == "" {
			http.Error(w, "Missing name or ID", http.StatusBadRequest)
			return
		}

		var entities []any
		if id, err := strconv.Atoi(part); err == nil {
			if e, err := fromID(db, table, id); err == nil {
				entities = append(entities, e)
			}
		} else if isAlphabetical(part) {
			if ents, err := FromName(db, table, part); err == nil {
				entities = append(entities, ents...)
			}
		} else {
			http.Error(w, "Invalid query format", http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{table: entities})
	}
}

// --- Fuzzy search by name ---
func FromName(db *sql.DB, table, name string) ([]any, error) {
	//TODO make this less crude.
	nameUpper := strings.ToUpper(name)
	var results []any

	switch table {
	case "items":
		ids, err := getIDsByName(db, "items", nameUpper)
		if err != nil {
			return nil, err
		}
		for _, id := range ids {
			if e, err := getItem(db, id); err == nil {
				results = append(results, e)
			}
		}

	case "mercs":
		ids, err := getIDsByName(db, "mercs", nameUpper)
		if err != nil {
			return nil, err
		}
		for _, id := range ids {
			if e, err := getMerc(db, id); err == nil {
				results = append(results, e)
			}
		}

	case "spells":
		ids, err := getIDsByName(db, "spells", nameUpper)
		if err != nil {
			return nil, err
		}
		for _, id := range ids {
			if e, err := getSpell(db, id); err == nil {
				results = append(results, e)
			}
		}

	case "units":
		ids, err := getIDsByName(db, "units", nameUpper)
		if err != nil {
			return nil, err
		}
		for _, id := range ids {
			if e, err := getUnit(db, id); err == nil {
				results = append(results, e)
			}
		}

	case "sites":
		ids, err := getIDsByName(db, "sites", nameUpper)
		if err != nil {
			return nil, err
		}
		for _, id := range ids {
			if e, err := getSite(db, id); err == nil {
				results = append(results, e)
			}
		}

	default:
		return nil, fmt.Errorf("unknown table: %s", table)
	}

	return results, nil
}

// --- Helper to find IDs by fuzzy name ---
func getIDsByName(db *sql.DB, table, nameUpper string) ([]int, error) {
	//TODO make sure that this is working properly
	rows, err := db.Query("SELECT id, name FROM " + table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []int
	for rows.Next() {
		var id int
		var rowName string
		if err := rows.Scan(&id, &rowName); err != nil {
			continue
		}
		rowUpper := strings.ToUpper(rowName)
		score := fuzzy.Ratio(nameUpper, rowUpper)
		partial := fuzzy.PartialRatio(nameUpper, rowUpper)
		if score >= 70 || partial >= 85 {
			ids = append(ids, id)
		}
	}
	return ids, nil
}

// --- ID lookup ---
func fromID(db *sql.DB, table string, id int) (any, error) {
	switch table {
	case "items":
		return getItem(db, id)
	case "mercs":
		return getMerc(db, id)
	case "spells":
		return getSpell(db, id)
	case "units":
		return getUnit(db, id)
	case "sites":
		return getSite(db, id)
	default:
		return nil, fmt.Errorf("unknown table: %s", table)
	}
}

// --- Sub-functions ---

func getItem(db *sql.DB, id int) (*Item, error) {
	var i Item
	err := db.QueryRow(`
		SELECT id, name, type, constlevel, mainlevel, mpath, gemcost
		FROM items WHERE id=?`, id).Scan(
		&i.ID, &i.Name, &i.Type, &i.ConstLevel, &i.MainLevel, &i.MPath, &i.GemCost,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("item not found")
		}
		return nil, err
	}
	i.Table = "items"
	i.Screenshot = fmt.Sprintf("/data/Item/%d.png", i.ID)
	return &i, nil
}

func getMerc(db *sql.DB, id int) (*Merc, error) {
	var m Merc
	err := db.QueryRow(`
		SELECT id, name, bossname, commander_id, unit_id, nrunits
		FROM mercs WHERE id=?`, id).Scan(
		&m.ID, &m.Name, &m.BossName, &m.CommanderID, &m.UnitID, &m.NRUnits,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("merc not found")
		}
		return nil, err
	}
	m.Table = "mercs"
	m.Screenshot = fmt.Sprintf("/data/Merc/%d.png", m.ID)
	return &m, nil
}

func getSpell(db *sql.DB, id int) (*Spell, error) {
	var s Spell
	err := db.QueryRow(`
		SELECT id, name, gemcost, mpath, type, school, researchlevel
		FROM spells WHERE id=?`, id).Scan(
		&s.ID, &s.Name, &s.GemCost, &s.MPath, &s.Type, &s.School, &s.ResearchLevel,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("spell not found")
		}
		return nil, err
	}
	s.Table = "spells"
	s.Screenshot = fmt.Sprintf("/data/Spell/%d.png", s.ID)
	return &s, nil
}

func getUnit(db *sql.DB, id int) (*Unit, error) {
	var u Unit
	err := db.QueryRow(`
		SELECT id, name, hp, size
		FROM units WHERE id=?`, id).Scan(&u.ID, &u.Name, &u.HP, &u.Size)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("unit not found")
		}
		return nil, err
	}
	u.Table = "units"
	u.Screenshot = fmt.Sprintf("/data/Unit/%d.png", u.ID)
	return &u, nil
}

func getSite(db *sql.DB, id int) (*Site, error) {
	var s Site
	err := db.QueryRow(`
		SELECT id, name, path, level, rarity
		FROM sites WHERE id=?`, id).Scan(&s.ID, &s.Name, &s.Path, &s.Level, &s.Rarity)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("site not found")
		}
		return nil, err
	}
	s.Table = "sites"
	s.Screenshot = fmt.Sprintf("/data/Site/%d.png", s.ID)
	return &s, nil
}

func main() {
	tables := []string{"item", "spell", "unit", "site", "merc", "event"}
	db := DBcheck("dom6api.db", "create_tables.sql")
	defer db.Close()

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

func isAlphabetical(s string) bool {
	for _, r := range s {
		if r < 'A' || r > 'Z' {
			return false
		}
	}
	return true
}

// TODO re-add all the edgecases from the previous app
// TODO add mount edgcases. Mount, co-rider
// TODO add glamour
// TODO trim
// TODO figure out what this function was for
func NewUnit(props map[string][]string) *Unit {
	if paths, ok := props["randompaths"]; ok {
		decoded := make([]string, len(paths))
		copy(decoded, paths)
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
