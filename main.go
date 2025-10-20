package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"

	fuzzy "github.com/paul-mannino/go-fuzzywuzzy"
	_ "modernc.org/sqlite"
)

const (
	DBFile = "Data/dom6api.db"

	ItemTable  = "items"
	MercTable  = "mercs"
	SpellTable = "spells"
	UnitTable  = "units"
	SiteTable  = "sites"

	fuzzyScoreThreshold   = 70
	fuzzyPartialThreshold = 85
)

var (
	cleanRe = regexp.MustCompile(`[^a-zA-Z0-9 ]+`)
)

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

		clean := cleanRe.ReplaceAllString(part, "")
		if clean == "" {
			http.Error(w, "Invalid query format", http.StatusBadRequest)
			return
		}

		var entities []any
		if id, err := strconv.Atoi(clean); err == nil {
			if e, err := fromID(db, table, id); err == nil {
				entities = append(entities, e)
			}
		} else {
			upperClean := strings.ToUpper(clean)
			ids, err := getIDsByName(db, table, upperClean)
			if err == nil && len(ids) > 0 {
				for _, id := range ids {
					if e, err := fromID(db, table, id); err == nil {
						entities = append(entities, e)
					}
				}
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{table: entities})
	}
}

// --- ID lookup ---
func fromID(db *sql.DB, table string, id int) (any, error) {
	switch table {

	case ItemTable:
		items, err := getItems(db, []int{id})
		ensureResult(items, err, "item not found")
		return items[0], nil

	case MercTable:
		mercs, err := getMercs(db, []int{id})
		ensureResult(mercs, err, "merc not found")
		return mercs[0], nil

	case SpellTable:
		spells, err := getSpells(db, []int{id})
		ensureResult(spells, err, "spell not found")
		return spells[0], nil

	case UnitTable:
		units, err := getUnits(db, []int{id})
		ensureResult(units, err, "unit not found")
		return units[0], nil

	case SiteTable:
		sites, err := getSites(db, []int{id})
		ensureResult(sites, err, "site not found")
		return sites[0], nil

	default:
		return nil, fmt.Errorf("unknown table: %s", table)
	}
}

// --- error helper condenser ---
func ensureResult[T any](objs []*T, err error, notFoundMsg string) ([]*T, error) {
	if err != nil {
		return nil, err
	}
	if len(objs) == 0 {
		return nil, fmt.Errorf("%s", notFoundMsg)
	}
	return objs, nil
}

// --- Sub-functions ---

// --- Helper to find IDs by fuzzy name ---
func getIDsByName(db *sql.DB, table, nameUpper string) ([]int, error) {
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
		if fuzzy.Ratio(nameUpper, rowUpper) >= fuzzyScoreThreshold ||
			fuzzy.PartialRatio(nameUpper, rowUpper) >= fuzzyPartialThreshold {
			ids = append(ids, id)
		}
	}
	return ids, nil
}

func fetchByIDs[T any](db *sql.DB, table string, ids []int, scanRow func(*sql.Rows) (T, error)) ([]*T, error) {
	if len(ids) == 0 {
		return nil, fmt.Errorf("no ids provided")
	}

	query := fmt.Sprintf("SELECT * FROM %s WHERE id IN (%s)", table, strings.TrimRight(strings.Repeat("?,", len(ids)), ","))
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*T
	for rows.Next() {
		if obj, err := scanRow(rows); err == nil {
			result = append(result, &obj)
		}
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return result, nil
}

// --- Condensed get functions ---
func getItems(db *sql.DB, ids []int) ([]*Item, error) {
	return fetchByIDs(db, ItemTable, ids, func(r *sql.Rows) (Item, error) {
		var i Item
		err := r.Scan(&i.ID, &i.Name, &i.Type, &i.ConstLevel, &i.MainLevel, &i.MPath, &i.GemCost)
		i.Table = ItemTable
		i.Screenshot = fmt.Sprintf("/data/Item/%d.png", i.ID)
		return i, err
	})
}

func getMercs(db *sql.DB, ids []int) ([]*Merc, error) {
	return fetchByIDs(db, MercTable, ids, func(r *sql.Rows) (Merc, error) {
		var m Merc
		err := r.Scan(&m.ID, &m.Name, &m.BossName, &m.CommanderID, &m.UnitID, &m.NRUnits)
		m.Table = MercTable
		m.Screenshot = fmt.Sprintf("/data/Merc/%d.png", m.ID)
		return m, err
	})
}

func getSpells(db *sql.DB, ids []int) ([]*Spell, error) {
	return fetchByIDs(db, SpellTable, ids, func(r *sql.Rows) (Spell, error) {
		var s Spell
		err := r.Scan(&s.ID, &s.Name, &s.GemCost, &s.MPath, &s.Type, &s.School, &s.ResearchLevel)
		s.Table = SpellTable
		s.Screenshot = fmt.Sprintf("/data/Spell/%d.png", s.ID)
		return s, err
	})
}

func getUnits(db *sql.DB, ids []int) ([]*Unit, error) {
	return fetchByIDs(db, UnitTable, ids, func(r *sql.Rows) (Unit, error) {
		var u Unit
		err := r.Scan(&u.ID, &u.Name, &u.HP, &u.Size)
		u.Table = UnitTable
		u.Screenshot = fmt.Sprintf("/data/Unit/%d.png", u.ID)
		return u, err
	})
}

func getSites(db *sql.DB, ids []int) ([]*Site, error) {
	return fetchByIDs(db, SiteTable, ids, func(r *sql.Rows) (Site, error) {
		var s Site
		err := r.Scan(&s.ID, &s.Name, &s.Path, &s.Level, &s.Rarity)
		s.Table = SiteTable
		s.Screenshot = fmt.Sprintf("/data/Site/%d.png", s.ID)
		return s, err
	})
}

func main() {
	tables := []string{"item", "spell", "unit", "site", "merc"}
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

Once confirmed, I’ll rewrite your Go handler layer to cover the same query behavior as the PHP routes.
*/
