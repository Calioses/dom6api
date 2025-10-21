package main

// import (
// 	"database/sql"
// 	"encoding/json"
// 	"fmt"
// 	"io/ioutil"
// 	"log"
// 	"os"
// 	"time"

// 	_ "github.com/mattn/go-sqlite3"
// 	"github.com/playwright-community/playwright-go"
// )

// func scrape() {
// 	pw, err := playwright.Run()
// 	if err != nil {
// 		log.Fatal(err)
// 	}
// 	browser, err := pw.Chromium.Launch()
// 	if err != nil {
// 		log.Fatal(err)
// 	}
// 	page, err := browser.NewPage()
// 	if err != nil {
// 		log.Fatal(err)
// 	}
// 	page.Goto("http://localhost:8000/?loadEvents=1")
// 	time.Sleep(500 * time.Millisecond)

// 	types := []string{"item", "spell", "unit", "site", "merc", "event"}
// 	for _, typ := range types {
// 		page.Evaluate(fmt.Sprintf(`$('#%s-page-button').trigger('click');`, typ))
// 		time.Sleep(500 * time.Millisecond)

// 		numEntities, _ := page.Evaluate(fmt.Sprintf(`DMI.modctx.%sdata.length`, typ))
// 		n := int(numEntities.(float64))

// 		for i := 0; i < n; i++ {
// 			entityID, _ := page.Evaluate(fmt.Sprintf(`
// 				(()=>{
// 					const entity = DMI.modctx.%sdata[%d];
// 					$('#%s-page div.fixed-overlay').empty().append(entity.renderOverlay(entity));
// 					return entity.id;
// 				})()
// 			`, typ, i, typ))

// 			time.Sleep(500 * time.Millisecond)
// 			locator := page.Locator(fmt.Sprintf("#%s-page div.fixed-overlay", typ))
// 			os.MkdirAll(fmt.Sprintf("../data/screenshot/%s", typ), 0755)
// 			path := fmt.Sprintf("../data/screenshot/%s/%v.png", typ, entityID)
// 			locator.Screenshot(playwright.ElementHandleScreenshotOptions{
// 				Path: playwright.String(path),
// 			})

// 		}
// 	}

// 	populateItemsDB(page)
// 	populateSpellsDB(page)
// 	populateUnitsDB(page)
// 	populateMercsDB(page)
// 	populateSitesDB(page)

// 	browser.Close()
// 	pw.Stop()
// }

// // -------------------- ITEMS --------------------
// func populateItemsDB(page playwright.Page) {
// 	db, _ := sql.Open("sqlite3", "../data/items.db")
// 	defer db.Close()
// 	db.Exec("DROP TABLE IF EXISTS items")
// 	sqlContent, _ := ioutil.ReadFile("items.sql")
// 	db.Exec(string(sqlContent))

// 	itemsRaw, _ := page.Evaluate(`DMI.modctx.itemdata`)
// 	itemsJSON, _ := json.Marshal(itemsRaw)
// 	var items []map[string]interface{}
// 	json.Unmarshal(itemsJSON, &items)

// 	stmt, _ := db.Prepare("INSERT INTO items (id, name, type, constlevel, mainlevel, mpath, gemcost) VALUES (?, ?, ?, ?, ?, ?, ?)")
// 	defer stmt.Close()
// 	for _, item := range items {
// 		stmt.Exec(item["id"], item["name"], item["type"], item["constlevel"], item["mainlevel"], item["mpath"], item["gemcost"])
// 	}
// }

// // -------------------- SPELLS --------------------
// func populateSpellsDB(page playwright.Page) {
// 	db, _ := sql.Open("sqlite3", "../data/spells.db")
// 	defer db.Close()
// 	db.Exec("DROP TABLE IF EXISTS spells")
// 	sqlContent, _ := ioutil.ReadFile("spells.sql")
// 	db.Exec(string(sqlContent))

// 	spellsRaw, _ := page.Evaluate(`DMI.modctx.spelldata.filter(spell => spell.research != "unresearchable")`)
// 	spellsJSON, _ := json.Marshal(spellsRaw)
// 	var spells []map[string]interface{}
// 	json.Unmarshal(spellsJSON, &spells)

// 	stmt, _ := db.Prepare("INSERT INTO spells (id, name, gemcost, mpath, type, school, researchlevel) VALUES (?, ?, ?, ?, ?, ?, ?)")
// 	defer stmt.Close()
// 	schools := []string{"Conjuration", "Alteration", "Evocation", "Construction", "Enchantment", "Thaumaturgy", "Blood", "Divine"}
// 	for _, s := range spells {
// 		school := int(s["school"].(float64))
// 		stmt.Exec(s["id"], s["name"], s["gemcost"], s["mpath"], s["type"], schools[school], s["researchlevel"])
// 	}
// }

// // -------------------- UNITS --------------------
// func populateUnitsDB(page playwright.Page) {
// 	db, _ := sql.Open("sqlite3", "../data/units.db")
// 	defer db.Close()
// 	sqlContent, _ := ioutil.ReadFile("units.sql")
// 	for _, q := range splitSQL(string(sqlContent)) {
// 		db.Exec(q)
// 	}

// 	unitsRaw, _ := page.Evaluate(`DMI.modctx.unitdata.filter(unit => Number.isInteger(unit.id))`)
// 	unitsJSON, _ := json.Marshal(unitsRaw)
// 	var units []map[string]interface{}
// 	json.Unmarshal(unitsJSON, &units)

// 	stmt, _ := db.Prepare("INSERT INTO units (id, name, hp, size) VALUES (?, ?, ?, ?)")
// 	defer stmt.Close()
// 	for _, u := range units {
// 		stmt.Exec(u["id"], u["fullname"], u["hp"], u["size"])
// 	}

// 	scalarProps := []string{"immobile", "mpath"}
// 	arrayProps := []string{}
// 	arrayJSONProps := []string{"randompaths"}
// 	excludedProps := []string{"id", "name", "hp", "size"}
// 	unreadableProps := []string{"armor", "cheapgod20", "cheapgod40", "createdby", "dupes", "eracodes", "nations", "recruitedby", "sprite", "summonedby", "summonedfrom", "weapons"}

// 	populatePropsTable(page, db, "unit", len(units), scalarProps, arrayProps, arrayJSONProps, excludedProps, unreadableProps)
// }

// // -------------------- MERCS --------------------
// func populateMercsDB(page playwright.Page) {
// 	db, _ := sql.Open("sqlite3", "../data/mercs.db")
// 	defer db.Close()
// 	db.Exec("DROP TABLE IF EXISTS mercs")
// 	sqlContent, _ := ioutil.ReadFile("mercs.sql")
// 	db.Exec(string(sqlContent))

// 	mercsRaw, _ := page.Evaluate(`DMI.modctx.mercdata`)
// 	mercsJSON, _ := json.Marshal(mercsRaw)
// 	var mercs []map[string]interface{}
// 	json.Unmarshal(mercsJSON, &mercs)

// 	stmt, _ := db.Prepare("INSERT INTO mercs (id, name, bossname, commander_id, unit_id, nrunits) VALUES (?, ?, ?, ?, ?, ?)")
// 	defer stmt.Close()
// 	for _, m := range mercs {
// 		stmt.Exec(m["id"], m["name"], m["bossname"], m["com"], m["unit"], m["nrunits"])
// 	}
// }

// // -------------------- SITES --------------------
// func populateSitesDB(page playwright.Page) {
// 	db, _ := sql.Open("sqlite3", "../data/sites.db")
// 	defer db.Close()
// 	sqlContent, _ := ioutil.ReadFile("sites.sql")
// 	for _, q := range splitSQL(string(sqlContent)) {
// 		db.Exec(q)
// 	}

// 	sitesRaw, _ := page.Evaluate(`DMI.modctx.sitedata`)
// 	sitesJSON, _ := json.Marshal(sitesRaw)
// 	var sites []map[string]interface{}
// 	json.Unmarshal(sitesJSON, &sites)

// 	stmt, _ := db.Prepare("INSERT INTO sites (id, name, path, level, rarity) VALUES (?, ?, ?, ?, ?)")
// 	defer stmt.Close()
// 	rarities := map[float64]string{0: "Common", 1: "Uncommon", 2: "Rare", 5: "Never random", 11: "Throne lvl1", 12: "Throne lvl2", 13: "Throne lvl3"}
// 	for _, s := range sites {
// 		r := s["rarity"].(float64)
// 		stmt.Exec(s["id"], s["name"], s["path"], s["level"], rarities[r])
// 	}

// 	scalarProps := []string{"F", "A", "W", "E", "S", "D", "N", "B", "G"}
// 	arrayProps := []string{"com", "futurenations", "hcom", "hmon", "mon", "nations", "provdef", "scales", "sum"}
// 	excludedProps := []string{"id", "name", "path", "level", "rarity", "renderOverlay", "matchProperty", "searchable", "listed_gempath", "mpath2", "scale1", "scale2", "sprite", "url"}
// 	populatePropsTable(page, db, "site", len(sites), scalarProps, arrayProps, []string{}, excludedProps, []string{})
// }

// // -------------------- PROPS --------------------
// func populatePropsTable(page playwright.Page, db *sql.DB, category string, numEntities int, scalarProps, arrayProps, arrayJSONProps, excludedProps, unreadableProps []string) {
// 	stmt, _ := db.Prepare(fmt.Sprintf("INSERT INTO %s_props (%s_id, prop_name, arrayprop_ix, value) VALUES (?, ?, ?, ?)", category, category))
// 	defer stmt.Close()

// 	for i := 0; i < numEntities; i++ {
// 		propsRaw, _ := page.Evaluate(fmt.Sprintf(`
// 			(()=>{
// 				const e = DMI.modctx.%sdata[%d];
// 				%s
// 				return e;
// 			})()
// 		`, category, i, buildDeletePropsJS(unreadableProps)))
// 		propsJSON, _ := json.Marshal(propsRaw)
// 		var props map[string]interface{}
// 		json.Unmarshal(propsJSON, &props)

// 		id := props["id"]

// 		for k, v := range props {
// 			if contains(excludedProps, k) {
// 				continue
// 			} else if contains(arrayProps, k) || contains(arrayJSONProps, k) {
// 				arr, ok := v.([]interface{})
// 				if !ok {
// 					continue
// 				}
// 				for ix, val := range arr {
// 					valStr := val
// 					if contains(arrayJSONProps, k) {
// 						js, _ := json.Marshal(val)
// 						valStr = string(js)
// 					}
// 					stmt.Exec(id, k, ix, valStr)
// 				}
// 			} else if contains(scalarProps, k) {
// 				stmt.Exec(id, k, nil, v)
// 			}
// 		}
// 	}
// }

// // -------------------- HELPERS --------------------
// func splitSQL(sqlContent string) []string {
// 	raw := []rune(sqlContent)
// 	var queries []string
// 	var buf []rune
// 	for _, r := range raw {
// 		if r == ';' {
// 			queries = append(queries, string(buf))
// 			buf = []rune{}
// 		} else {
// 			buf = append(buf, r)
// 		}
// 	}
// 	if len(buf) > 0 {
// 		queries = append(queries, string(buf))
// 	}
// 	return queries
// }

// func buildDeletePropsJS(props []string) string {
// 	js := ""
// 	for _, p := range props {
// 		js += fmt.Sprintf("delete e['%s'];", p)
// 	}
// 	return js
// }

// func contains(arr []string, val string) bool {
// 	for _, a := range arr {
// 		if a == val {
// 			return true
// 		}
// 	}
// 	return false
// }
