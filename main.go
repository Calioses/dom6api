package main

import (
	"database/sql"
	_ "embed"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

const (
	DBFile        = "Data/dom6api.db"
	InspectorPort = 8001
	APIPort       = 8002
)

//go:embed create_tables.sql
var sqlFile []byte

func dbcheck(filename string) *sql.DB {

	for _, cat := range []string{"events", "items", "mercs", "sites", "spells", "units"} {
		if err := os.MkdirAll(filepath.Join("Data", cat), os.ModePerm); err != nil {
			log.Fatalf("could not create folder %s: %v", cat, err)
		}
	}

	log.Println("Opening database:", filename)
	db, err := sql.Open("sqlite3", filename)
	if err != nil {
		log.Fatalf("dbcheck: failed to open database: %v", err)
	}

	if _, err := db.Exec(string(sqlFile)); err != nil {
		log.Fatalf("dbcheck: failed to execute SQL: %v", err)
	}

	log.Println("dbcheck: SQL executed successfully")
	return db
}

func main() {
	fmt.Println(`
______ ________  ___   ____    ___  ______ _____ 
|  _  \  _  |  \/  |  / ___|  / _ \ | ___ \_   _|
| | | | | | | .  . | / /___  / /_\ \| |_/ / | |  
| | | | | | | |\/| | | ___ \ |  _  ||  __/  | |  
| |/ /\ \_/ / |  | | | \_/ | | | | || |    _| |_ 
|___/  \___/\_|  |_/ \_____/ \_| |_/\_|    \___/ 
                                                 
MIT License
Use, modify, and distribute freely, with credit to Monkeydew — the G.O.A.T.
                                                 `)
	db := dbcheck(DBFile)
	defer db.Close()

	if len(os.Args) > 1 && os.Args[1] == "build" {
		folder := "dom6inspector"
		os.RemoveAll(folder)
		if err := exec.Command("git", "clone", "https://github.com/larzm42/dom6inspector", folder).Run(); err != nil {
			log.Fatal("Clone failed:", err)
		}

		pyCmd := exec.Command("python", "-m", "http.server", fmt.Sprint(InspectorPort))
		pyCmd.Dir, pyCmd.Stdout, pyCmd.Stderr = folder, os.Stdout, os.Stderr
		if err := pyCmd.Start(); err != nil {
			log.Fatal("Python server failed:", err)
		}
		log.Printf("Python server at http://localhost:%d (PID %d)", InspectorPort, pyCmd.Process.Pid)
		go pyCmd.Wait()

		for i := 0; i < 10; i++ {
			if resp, err := http.Get(fmt.Sprintf("http://localhost:%d", InspectorPort)); err == nil {
				resp.Body.Close()
				break
			}
			time.Sleep(500 * time.Millisecond)
		}

		Scrape()
		log.Println("Scrape complete.")
	}

	log.Printf("Starting Go server on http://0.0.0.0:%d ...", APIPort)
	if err := StartServer(DBFile, fmt.Sprintf("0.0.0.0:%d", APIPort)); err != nil {
		log.Fatal("Go server failed:", err)
	}
}
