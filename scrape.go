package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
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
	schoolMap = map[float64]string{1: "Conjuration", 2: "Alteration", 3: "Evocation", 4: "Construction", 5: "Enchantment", 6: "Thaumaturgy", 7: "Blood", 8: "Divine"}
	rarities  = map[int]string{0: "Common", 1: "Uncommon", 2: "Rare", 5: "Never random", 11: "Throne lvl1", 12: "Throne lvl2", 13: "Throne lvl3"}
)

func dbcheck(filename string, sqlFile string) *sql.DB {
	base := "Data"
	categories := []string{"events", "items", "mercs", "sites", "spells", "units"}

	for _, cat := range categories {
		path := filepath.Join(base, cat)
		if err := os.MkdirAll(path, os.ModePerm); err != nil {
			log.Fatalf("could not create folder %s: %v", path, err)
		}
	}

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

	db, err := sql.Open("sqlite3", "Data/dom6api.db")
	if err != nil {
		log.Fatalf("main: could not open db: %v", err)
	}
	defer db.Close()
	// types     = []string{"item", "spell", "unit", "site", "merc", "event"}

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
		result, _ := page.Evaluate(exprCount)
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
			entityAny, err := page.Evaluate(fmt.Sprintf(`(i) => {
				const e = DMI.modctx['%sdata'][i];
				const container = document.querySelector('#%s-page div.fixed-overlay');
				container.innerHTML = e.renderOverlay(e).outerHTML || e.renderOverlay(e);
				return { e, ready: container.innerHTML.trim().length > 0 };
			}`, t, t), i)
			if err != nil {
				log.Printf("main: could not render entity %d for %s: %v", i, t, err)
				continue
			}

			entityMap := entityAny.(map[string]interface{})["e"].(map[string]interface{})
			if !entityAny.(map[string]interface{})["ready"].(bool) {
				log.Printf("main: overlay not ready for %s id %v", t, entityMap["id"])
				continue
			}

			entityID := fmt.Sprintf("%v", entityMap["id"])
			overlay, _ := page.QuerySelector(selector)

			path := filepath.Join("Data", t+"s", entityID+".png")
			for attempt := 1; attempt <= 3; attempt++ {
				if _, err := overlay.Screenshot(playwright.ElementHandleScreenshotOptions{Path: playwright.String(path)}); err == nil {
					break
				}
			}

			populate(page, db, t, i) // reuse open DB

			log.Printf("[%s] Rendering entity %d/%d...", t, i+1, count)
		}
	}

	log.Println("main: Done. Closing browser...")
}

func populate(page playwright.Page, db *sql.DB, category string, entityIndex int) {

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
			entityMap["gemcost"] = "0"
		}

		v, ok := entityMap["school"].(string)
		if !ok || v == "" || v == "-1" {
			return
		}
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			if school := schoolMap[f]; school != "" {
				entityMap["school"] = school
			} else {
				return
			}
		} else {
			entityMap["school"] = v
		}

	case "site":
		if f, ok := entityMap["rarity"].(float64); ok {
			if mapped, exists := rarities[int(f)]; exists {
				entityMap["rarity"] = mapped
			}
		}

	case "unit":
		idVal, ok := entityMap["id"]
		if !ok {
			log.Printf("populate: skipping unit, missing id")
			return
		}
		id, err := strconv.Atoi(fmt.Sprintf("%v", idVal))
		if err != nil {
			log.Printf("populate: skipping unit, invalid id: %v", idVal)
			return
		}

		name, _ := entityMap["name"].(string)

		hp, _ := strconv.Atoi(fmt.Sprintf("%v", entityMap["hp"]))
		size, _ := strconv.Atoi(fmt.Sprintf("%v", entityMap["size"]))
		mount, _ := strconv.Atoi(fmt.Sprintf("%v", entityMap["mount"]))
		coRider, _ := strconv.Atoi(fmt.Sprintf("%v", entityMap["co_rider"]))

		_, err = db.Exec(
			"INSERT OR REPLACE INTO units (id, name, hp, size, mount, co_rider) VALUES (?, ?, ?, ?, ?, ?)",
			id, name, hp, size, mount, coRider,
		)
		if err != nil {
			log.Printf("populate: failed to insert unit %d: %v", id, err)
		}

	}

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
		values[i] = fmt.Sprintf("%v", entityMap[f])
	}
	if _, err := stmt.Exec(values...); err != nil {
		log.Printf("populate: exec error for %s: %v", category, err)
	}
}
