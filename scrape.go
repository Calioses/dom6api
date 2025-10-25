package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

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

	rarities = map[int]string{
		0:  "Common",
		1:  "Uncommon",
		2:  "Rare",
		5:  "Never random",
		11: "Throne lvl1",
		12: "Throne lvl2",
		13: "Throne lvl3",
	}

	categoryFields = map[string][]string{
		// "item":  {"id", "name", "type", "constlevel", "mainlevel", "mpath", "gemcost"},
		// "spell": {"id", "name", "gemcost", "mpath", "type", "school", "researchlevel"},
		"unit": {"id", "name", "hp", "size", "mountmnr", "coridermnr"},
		// "merc":  {"id", "name", "bossname", "com", "unit", "nrunits"},
		// "site":  {"id", "name", "path", "level", "rarity"},
		// "event": {"id", "name"},
	}
)

// TODO add a slowdown rendering isn't finishing
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
	os.Exit(0)
}

func scrape() {
	dbcheck(DBFile, SqlFile)

	pw, err := playwright.Run()
	if err != nil {
		log.Fatalf("main: could not start Playwright: %v", err)
	}

	browser, err := pw.Chromium.Launch(playwright.BrowserTypeLaunchOptions{Headless: playwright.Bool(true)})
	if err != nil {
		log.Fatalf("main: could not launch browser: %v", err)
	}

	page, err := browser.NewPage()
	if err != nil {
		log.Fatalf("main: could not create page: %v", err)
	}
	page.SetViewportSize(800, 600)

	if _, err = page.Goto("http://localhost:8001/?loadEvents=1", playwright.PageGotoOptions{
		WaitUntil: playwright.WaitUntilStateNetworkidle,
	}); err != nil {
		log.Fatalf("main: could not goto url: %v", err)
	}

	db, err := sql.Open("sqlite3", "Data/dom6api.db")
	if err != nil {
		log.Fatalf("main: could not open db: %v", err)
	}

	for cat := range categoryFields {
		log.Printf("Processing category: %s", cat)

		fmt.Printf("Clicking page button for category %s\n", cat)
		page.Click(fmt.Sprintf("#%s-page-button", cat))

		fmt.Printf("Waiting for data for category %s\n", cat)
		page.WaitForFunction(fmt.Sprintf(`
        () => DMI.modctx['%sdata'] && DMI.modctx['%sdata'].length > 0
    `, cat, cat), playwright.PageWaitForFunctionOptions{Timeout: playwright.Float(10000)})

		countAny, _ := page.Evaluate(fmt.Sprintf(`() => DMI.modctx['%sdata'].length`, cat))
		count := int(countAny.(int))
		selector := fmt.Sprintf("#%s-page div.fixed-overlay", cat)

		for i := 0; i < count; i++ {
			fmt.Printf("Rendering entity %d/%d for category %s\n", i+1, count, cat)
			var entity interface{}
			var err error

			for retries := 0; retries < 5; retries++ {
				entity, err = page.Evaluate(fmt.Sprintf(`(i)=>{
				const e = DMI.modctx['%sdata'][i];
				const o = document.querySelector('#%s-page div.fixed-overlay');
				return new Promise(resolve=>{
					const f = ()=>{
						const rendered = e.renderOverlay(e);
						o.innerHTML = rendered?.outerHTML || "";
						if(o.innerHTML.trim().length > 0) resolve(e);
						else setTimeout(f, 200);
					};
					f();
				});
			}`, cat, cat), i)

				if err == nil {
					break
				}
				fmt.Printf("Retrying render for entity %d: %v\n", i, err)
				time.Sleep(500 * time.Millisecond)
			}

			if err != nil {
				fmt.Printf("Failed to render entity %d after retries: %v\n", i, err)
				continue
			}

			entityMap, ok := entity.(map[string]interface{})
			if !ok {
				fmt.Printf("Entity %d not a map, skipping\n", i)
				time.Sleep(200 * time.Millisecond)
				continue
			}

			path := filepath.Join("Data", cat+"s", fmt.Sprintf("%v.png", entityMap["id"]))
			if el, err := page.QuerySelector(selector); err == nil && el != nil {
				fmt.Printf("Taking screenshot for entity %d, saving to %s\n", i, path)
				el.Screenshot(playwright.ElementHandleScreenshotOptions{Path: playwright.String(path)})
			}

			fmt.Printf("Populating database for entity %d/%d\n", i+1, count)
			populate(page, db, cat, i)
			fmt.Printf("[%s] Rendered entity %d/%d\n", cat, i+1, count)
			time.Sleep(100 * time.Millisecond)
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
		if r, ok := entityMap["rarity"].(int); ok {
			entityMap["rarity"] = rarities[r]
		}

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

	values := make([]interface{}, len(fields))
	for i, f := range fields {
		switch v := entityMap[f].(type) {
		case float64:
			values[i] = int(v)
		default:
			values[i] = v
		}
	}

	if _, err := stmt.Exec(values...); err != nil {
		log.Printf("populate: exec error for %s: %v", category, err)
	}
}
