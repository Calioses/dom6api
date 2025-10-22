package main

import (
	"fmt"
	"log"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3"
	"github.com/playwright-community/playwright-go"
)

const (
	DBFile        = "Data/dom6api.db"
	SqlFile       = "create_tables.sql"
	InspectorPort = 8001
	APIPort       = 8002
)

func main() {
	log.Println("Starting Playwright...")
	pw, err := playwright.Run()
	if err != nil {
		log.Fatalf("could not start Playwright: %v", err)
	}
	defer pw.Stop()

	log.Println("Launching Chromium...")
	browser, err := pw.Chromium.Launch(playwright.BrowserTypeLaunchOptions{
		Headless: playwright.Bool(true),
	})
	if err != nil {
		log.Fatalf("could not launch browser: %v", err)
	}
	defer browser.Close()

	log.Println("Creating new page...")
	page, err := browser.NewPage()
	if err != nil {
		log.Fatalf("could not create page: %v", err)
	}
	page.SetViewportSize(800, 600)

	url := "http://localhost:8001/?loadEvents=1"
	log.Printf("Navigating to %s", url)
	if _, err = page.Goto(url, playwright.PageGotoOptions{
		WaitUntil: playwright.WaitUntilStateNetworkidle,
	}); err != nil {
		log.Fatalf("could not goto url %s: %v", url, err)
	}

	types := []string{"item", "spell", "unit", "site", "merc", "event"}

	for _, t := range types {
		log.Printf("Clicking tab for %s...", t)
		if _, err := page.Evaluate(fmt.Sprintf(`() => { document.getElementById('%s-page-button').click(); }`, t)); err != nil {
			log.Fatalf("could not click tab for %s: %v", t, err)
		}

		selector := fmt.Sprintf("#%s-page div.fixed-overlay", t)
		log.Printf("Waiting for overlay selector: %s", selector)
		if _, err := page.WaitForSelector(selector); err != nil {
			log.Fatalf("overlay selector not found for %s: %v", t, err)
		}

		exprCount := fmt.Sprintf(`() => DMI.modctx['%sdata'].length`, t)
		result, err := page.Evaluate(exprCount)
		if err != nil {
			log.Fatalf("could not evaluate count for %s: %v", t, err)
		}

		var count int
		switch v := result.(type) {
		case float64:
			count = int(v)
		case int:
			count = v
		default:
			log.Fatalf("unexpected type for count: %T", v)
		}
		log.Printf("%d entities found for %s", count, t)

		for i := 0; i < count; i++ {
			log.Printf("[%s] Rendering entity %d/%d...", t, i+1, count)

			exprRender := fmt.Sprintf(`(i) => {
				const e = DMI.modctx['%sdata'][i];
				const container = document.querySelector('#%s-page div.fixed-overlay');
				container.innerHTML = e.renderOverlay(e).outerHTML || e.renderOverlay(e);
				return e.id;
			}`, t, t)

			idAny, err := page.Evaluate(exprRender, i)
			if err != nil {
				log.Printf("could not render overlay for %s index %d, skipping: %v", t, i, err)
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
				log.Printf("overlay did not render for %s id %s, skipping: %v", t, entityID, err)
				continue
			}

			overlay, err := page.QuerySelector(selector)
			if err != nil || overlay == nil {
				log.Printf("overlay element missing for %s id %s, skipping: %v", t, entityID, err)
				continue
			}

			path := filepath.Join("Data", t+"s", entityID+".png")

			// Retry screenshot up to 3 times
			success := false
			for attempt := 1; attempt <= 3; attempt++ {
				if _, err := overlay.Screenshot(playwright.ElementHandleScreenshotOptions{
					Path: playwright.String(path),
				}); err != nil {
					log.Printf("Attempt %d: could not screenshot overlay for %s id %s: %v", attempt, t, entityID, err)
				} else {
					success = true
					break
				}
			}
			if !success {
				log.Printf("Skipping %s id %s after 3 failed attempts", t, entityID)
				continue
			}

			log.Printf("Screenshot saved for %s id %s", t, entityID)

			// small delay to avoid rate limiting
			// time.Sleep(150 * time.Millisecond)
		}
	}

	log.Println("Done. Closing browser...")
}
