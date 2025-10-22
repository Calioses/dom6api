package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"path/filepath"
	"slices"
	"strings"

	_ "github.com/mattn/go-sqlite3"
	"github.com/playwright-community/playwright-go"
)

var (
	schools        = []string{"Conjuration", "Alteration", "Evocation", "Construction", "Enchantment", "Thaumaturgy", "Blood", "Divine"}
	rarities       = map[int]string{0: "Common", 1: "Uncommon", 2: "Rare", 5: "Never random", 11: "Throne lvl1", 12: "Throne lvl2", 13: "Throne lvl3"}
	jsonArrayProps = []string{"randompaths"}
	unitExcluded   = map[string]struct{}{"id": {}, "fullname": {}, "hp": {}, "size": {}, "mount": {}, "co_rider": {}}
	siteExcluded   = map[string]struct{}{"id": {}, "name": {}, "path": {}, "level": {}, "rarity": {}}
)

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

	var types = []string{"item", "spell", "unit", "site", "merc", "event"}

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
			if t == "spell" {
				skip, _ := page.Evaluate(fmt.Sprintf(`(i) => DMI.modctx['%sdata'][i].unresearchable || false`, t), i)
				if skip.(bool) {
					continue
				}
			}

			exprRender := fmt.Sprintf(`(i) => {
				const e = DMI.modctx['%sdata'][i];
				const container = document.querySelector('#%s-page div.fixed-overlay');
				container.innerHTML = e.renderOverlay(e).outerHTML || e.renderOverlay(e);
				return e.id;
			}`, t, t)

			idAny, err := page.Evaluate(exprRender, i)
			if err != nil {
				log.Printf("main: could not render overlay for %s index %d, skipping: %v", t, i, err)
				continue
			}
			entityID := fmt.Sprintf("%v", idAny)

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

			log.Printf("Screenshot saved for %s id %s", t, entityID)

			populate(page, "Data/dom6api.db", t, i)
		}
	}

	log.Println("main: Done. Closing browser...")
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
	entity := entityMap

	switch category {
	case "spell":
		if f, ok := entity["school"].(float64); ok && int(f) >= 0 && int(f) < len(schools) {
			entity["school"] = schools[int(f)]
		}
	case "site":
		if f, ok := entity["rarity"].(float64); ok {
			if r, exists := rarities[int(f)]; exists {
				entity["rarity"] = r
			}
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
		values[i] = entity[f]
	}
	if _, err := stmt.Exec(values...); err != nil {
		if strings.Contains(err.Error(), "CHECK constraint failed") && strings.Contains(err.Error(), "type") {
			log.Printf("populate: CHECK constraint failed for 'type' on item id %v, value passed: %v", entity["id"], entity["type"])
		} else {
			log.Printf("populate: exec error for %s: %v", category, err)
		}
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

		populateProps(stmtProps, entity["id"], entity, excluded)
	}
}
