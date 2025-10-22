package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	_ "github.com/mattn/go-sqlite3"
	"github.com/playwright-community/playwright-go"
)

const (
	DBFile        = "Data/dom6api.db"
	SqlFile       = "create_tables.sql"
	InspectorPort = 8001
	APIPort       = 8002
)

var (
	schoolMap = map[float64]string{
		1: "Conjuration",
		2: "Alteration",
		3: "Evocation",
		4: "Construction",
		5: "Enchantment",
		6: "Thaumaturgy",
		7: "Blood",
		8: "Divine",
	}
	rarities       = map[int]string{0: "Common", 1: "Uncommon", 2: "Rare", 5: "Never random", 11: "Throne lvl1", 12: "Throne lvl2", 13: "Throne lvl3"}
	jsonArrayProps = []string{"randompaths"}
	unitExcluded   = map[string]struct{}{"id": {}, "fullname": {}, "hp": {}, "size": {}, "mount": {}, "co_rider": {}}
	siteExcluded   = map[string]struct{}{"id": {}, "name": {}, "path": {}, "level": {}, "rarity": {}}
)

func dbcheck(filename string, sqlFile string) *sql.DB {
	log.Println("Opening database:", filename)
	db, err := sql.Open("sqlite3", filename)
	if err != nil {
		log.Fatalf("dbcheck: failed to open database: %v", err)
	}

	sqlBytes, err := os.ReadFile(sqlFile)
	if err != nil {
		log.Fatalf("dbcheck: failed to read SQL file: %v", err)
	}

	sqlStatements := string(sqlBytes)
	if _, err := db.Exec(sqlStatements); err != nil {
		log.Fatalf("dbcheck: failed to execute SQL file: %v", err)
	}

	log.Println("dbcheck: SQL script executed successfully.")
	return db
}
func main() {
	scrape()
}

func scrape() {
	dbcheck(DBFile, SqlFile)
	log.Println("Starting Playwright...")
	pw, err := playwright.Run()
	if err != nil {
		log.Fatalf("main: could not start Playwright: %v", err)
	}
	defer pw.Stop()

	log.Println("Launching Chromium...")
	browser, err := pw.Chromium.Launch(playwright.BrowserTypeLaunchOptions{
		Headless: playwright.Bool(true),
	})
	if err != nil {
		log.Fatalf("main: could not launch browser: %v", err)
	}
	defer browser.Close()

	log.Println("Creating new page...")
	page, err := browser.NewPage()
	if err != nil {
		log.Fatalf("main: could not create page: %v", err)
	}
	page.SetViewportSize(800, 600)

	url := "http://localhost:8001/?loadEvents=1"
	log.Printf("Navigating to %s", url)
	if _, err = page.Goto(url, playwright.PageGotoOptions{
		WaitUntil: playwright.WaitUntilStateNetworkidle,
	}); err != nil {
		log.Fatalf("main: could not goto url %s: %v", url, err)
	}

	// var types = []string{"item", "spell", "unit", "site", "merc", "event"}
	var types = []string{"unit"}

	for _, t := range types {
		log.Printf("Clicking tab for %s...", t)
		if _, err := page.Evaluate(fmt.Sprintf(`() => { document.getElementById('%s-page-button').click(); }`, t)); err != nil {
			log.Fatalf("main: could not click tab for %s: %v", t, err)
		}

		selector := fmt.Sprintf("#%s-page div.fixed-overlay", t)
		log.Printf("Waiting for overlay selector: %s", selector)
		if _, err := page.WaitForSelector(selector); err != nil {
			log.Fatalf("main: overlay selector not found for %s: %v", t, err)
		}

		exprCount := fmt.Sprintf(`() => DMI.modctx['%sdata'].length`, t)
		result, err := page.Evaluate(exprCount)
		if err != nil {
			log.Fatalf("main: could not evaluate count for %s: %v", t, err)
		}

		var count int
		switch v := result.(type) {
		case float64:
			count = int(v)
		case int:
			count = v
		default:
			log.Fatalf("main: unexpected type for count: %T", v)
		}
		log.Printf("%d entities found for %s", count, t)

		for i := 0; i < count; i++ {
			log.Printf("[%s] Rendering entity %d/%d...", t, i+1, count)

			entityAny, err := page.Evaluate(fmt.Sprintf(`(i) => {
			const e = DMI.modctx['%sdata'][i];
			const container = document.querySelector('#%s-page div.fixed-overlay');
			container.innerHTML = e.renderOverlay(e).outerHTML || e.renderOverlay(e);
			return e;
		}`, t, t), i)
			if err != nil {
				log.Printf("main: could not render/fetch entity %d for %s, skipping: %v", i, t, err)
				continue
			}

			entityMap, ok := entityAny.(map[string]interface{})
			if !ok {
				log.Printf("main: unexpected type for entity %d in %s", i, t)
				continue
			}

			entityID := fmt.Sprintf("%v", entityMap["id"])

			waitJS := fmt.Sprintf(`() => {
			const el = document.querySelector('#%s-page div.fixed-overlay');
			return el && el.innerHTML.trim().length > 0;
		}`, t)
			if _, err := page.WaitForFunction(waitJS, playwright.PageWaitForFunctionOptions{
				Timeout: playwright.Float(15000),
			}); err != nil {
				log.Printf("main: overlay did not render for %s id %s, skipping: %v", t, entityID, err)
				continue
			}

			overlay, err := page.QuerySelector(selector)
			if err != nil || overlay == nil {
				log.Printf("main: overlay element missing for %s id %s, skipping: %v", t, entityID, err)
				continue
			}

			path := filepath.Join("Data", t+"s", entityID+".png")
			success := false
			for attempt := 1; attempt <= 3; attempt++ {
				if _, err := overlay.Screenshot(playwright.ElementHandleScreenshotOptions{
					Path: playwright.String(path),
				}); err != nil {
					log.Printf("main: Attempt %d: could not screenshot overlay for %s id %s: %v", attempt, t, entityID, err)
				} else {
					success = true
					break
				}
			}
			if !success {
				log.Printf("main: Skipping %s id %s after 3 failed attempts", t, entityID)
				continue
			}

			populate(page, "Data/dom6api.db", t, i)
		}
	}

	log.Println("main: Done. Closing browser...")
}

func populate(page playwright.Page, dbFile string, category string, entityIndex int) {
	db, err := sql.Open("sqlite3", dbFile)
	if err != nil {
		log.Fatalf("populate: could not open db: %v", err)
	}
	defer db.Close()

	result, err := page.Evaluate(fmt.Sprintf("() => DMI.modctx['%sdata'][%d]", category, entityIndex))
	if err != nil {
		log.Printf("populate: could not fetch entity %d for %s: %v", entityIndex, category, err)
		return
	}

	entityMap, ok := result.(map[string]interface{})
	if !ok {
		log.Printf("populate: unexpected type for entity %d in %s", entityIndex, category)
		return
	}

	switch category {
	case "spell":
		if entityMap["gemcost"] == nil {
			entityMap["gemcost"] = 0
		}

		schoolVal, ok := entityMap["school"]
		if !ok {
			return
		}

		var school string
		skip := false
		switch v := schoolVal.(type) {
		case float64:
			switch {
			case v == -1:
				skip = true
			case schoolMap[v] != "":
				school = schoolMap[v]
			default:
				skip = true
			}
		case string:
			switch {
			case v == "-1" || v == "":
				skip = true
			default:
				f, err := strconv.ParseFloat(v, 64)
				if err == nil {
					if schoolMap[f] != "" {
						school = schoolMap[f]
					} else {
						skip = true
					}
				} else {
					school = v
				}
			}
		default:
			skip = true
		}

		if skip {
			log.Printf("Skipping spell ID %v due to invalid school", entityMap["id"])
			return
		}
		entityMap["school"] = school

		// normalize researchlevel to integer
		if rl, ok := entityMap["researchlevel"].(string); ok {
			if val, err := strconv.Atoi(rl); err == nil {
				entityMap["researchlevel"] = val
			} else {
				entityMap["researchlevel"] = 0
			}
		} else if _, ok := entityMap["researchlevel"].(float64); !ok {
			entityMap["researchlevel"] = 0
		}

	case "site":
		if f, ok := entityMap["rarity"].(float64); ok {
			if mapped, exists := rarities[int(f)]; exists {
				entityMap["rarity"] = mapped
			}
		}

	case "unit":
		// normalize numeric fields
		if hp, ok := entityMap["hp"].(string); ok {
			if val, err := strconv.Atoi(hp); err == nil {
				entityMap["hp"] = val
			} else {
				entityMap["hp"] = 0
			}
		}

		if size, ok := entityMap["size"].(string); ok {
			if val, err := strconv.Atoi(size); err == nil {
				entityMap["size"] = val
			} else {
				entityMap["size"] = 1
			}
		}

		if mount, ok := entityMap["mount"].(string); ok {
			if val, err := strconv.Atoi(mount); err == nil {
				entityMap["mount"] = val
			} else {
				entityMap["mount"] = 0
			}
		}

		if co, ok := entityMap["co_rider"].(string); ok {
			if val, err := strconv.Atoi(co); err == nil {
				entityMap["co_rider"] = val
			} else {
				entityMap["co_rider"] = 0
			}
		}

		log.Printf("[%s] Unit ID %v | Name: %v | HP: %v | Size: %v | Mount: %v | Co-rider: %v",
			category, entityMap["id"], entityMap["name"], entityMap["hp"], entityMap["size"], entityMap["mount"], entityMap["co_rider"])

	}

	// out, _ := json.MarshalIndent(entityMap, "", "  ")
	// log.Printf("[%s] Entity %d: %s", category, entityIndex, string(out))

	categoryFields := map[string][]string{
		"item":  {"id", "name", "type", "constlevel", "mainlevel", "mpath", "gemcost"},
		"spell": {"id", "name", "gemcost", "mpath", "type", "school", "researchlevel"},
		"unit":  {"id", "fullname", "hp", "size", "mount", "co_rider"},
		"merc":  {"id", "name", "bossname", "com", "unit", "nrunits"},
		"site":  {"id", "name", "path", "level", "rarity"},
		"event": {"id", "name"},
	}

	fields, ok := categoryFields[category]
	if !ok {
		log.Printf("populate: unknown category: %s", category)
		return
	}

	insertSQL := fmt.Sprintf(
		"INSERT OR REPLACE INTO %ss (%s) VALUES (%s)",
		category,
		strings.Join(fields, ", "),
		strings.Repeat("?, ", len(fields)-1)+"?",
	)

	stmt, err := db.Prepare(insertSQL)
	if err != nil {
		log.Printf("populate: could not prepare insert for %s: %v", category, err)
		return
	}
	defer stmt.Close()

	values := make([]interface{}, len(fields))
	for i, f := range fields {
		values[i] = entityMap[f]
	}
	if _, err := stmt.Exec(values...); err != nil {
		log.Printf("populate: exec error for %s: %v", category, err)
	}

	if category == "units" || category == "sites" {
		excluded := unitExcluded
		if category == "sites" {
			excluded = siteExcluded
		}
		stmtPropsSQL := fmt.Sprintf(
			"INSERT INTO %s_props (%s_id, prop_name, value, arrayprop_ix) VALUES (?, ?, ?, ?)",
			category, category,
		)
		stmtProps, err := db.Prepare(stmtPropsSQL)
		if err != nil {
			log.Printf("populate: could not prepare props insert for %s: %v", category, err)
			return
		}
		defer stmtProps.Close()
		populateProps(stmtProps, entityMap["id"], entityMap, excluded)
	}
}

func populateProps(stmt *sql.Stmt, id interface{}, entity map[string]interface{}, excluded map[string]struct{}) {
	for prop, val := range entity {
		if _, skip := excluded[prop]; skip {
			continue
		}
		switch v := val.(type) {
		case []interface{}:
			for i, elem := range v {
				outVal := fmt.Sprintf("%v", elem)
				if slices.Contains(jsonArrayProps, prop) {
					b, _ := json.Marshal(elem)
					outVal = string(b)
				}
				if _, err := stmt.Exec(id, prop, outVal, i); err != nil {
					log.Printf("populateProps: could not exec stmt for prop %s: %v", prop, err)
				}
			}
		default:
			if _, err := stmt.Exec(id, prop, fmt.Sprintf("%v", v), nil); err != nil {
				log.Printf("populateProps: could not exec stmt for prop %s: %v", prop, err)
			}
		}
	}
}
