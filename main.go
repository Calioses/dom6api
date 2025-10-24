package main

// import (
// 	_ "github.com/mattn/go-sqlite3"
// )

// const (
// 	DBFile        = "Data/dom6api.db"
// 	SqlFile       = "create_tables.sql"
// 	InspectorPort = 8001
// 	APIPort       = 8002
// )

// func dbcheck(filename string, sqlFile string) *sql.DB {
// 	base := "Data"
// 	categories := []string{"events", "items", "mercs", "sites", "spells", "units"}

// 	for _, cat := range categories {
// 		path := filepath.Join(base, cat)
// 		if err := os.MkdirAll(path, os.ModePerm); err != nil {
// 			log.Fatalf("could not create folder %s: %v", path, err)
// 		}
// 	}

// 	log.Println("Opening database:", filename)
// 	db, err := sql.Open("sqlite3", filename)
// 	if err != nil {
// 		log.Fatalf("dbcheck: failed to open database: %v", err)
// 	}

// 	sqlBytes, err := os.ReadFile(sqlFile)
// 	if err != nil {
// 		log.Fatalf("dbcheck: failed to read SQL file: %v", err)
// 	}

// 	sqlStatements := string(sqlBytes)
// 	if _, err := db.Exec(sqlStatements); err != nil {
// 		log.Fatalf("dbcheck: failed to execute SQL file: %v", err)
// 	}

// 	log.Println("dbcheck: SQL script executed successfully.")
// 	return db
// }

// func main() {
// 	db := dbcheck(DBFile, SqlFile)
// 	defer db.Close()

// 	// go func() {
// 	// 	log.Printf("Starting Go server on http://localhost:%d ...", APIPort)
// 	// 	if err := StartServer(DBFile, fmt.Sprintf(":%d", APIPort)); err != nil {
// 	// 		log.Fatal("Go server failed:", err)
// 	// 	}
// 	// }()
// 	// log.Println("Go server launch initiated.")

// 	folder := "dom6inspector"
// 	if _, err := os.Stat(folder); os.IsNotExist(err) {
// 		log.Println("Folder not found. Cloning repo...")
// 		if err := exec.Command("git", "clone", "https://github.com/larzm42/dom6inspector", folder).Run(); err != nil {
// 			log.Fatal("Failed to clone repo:", err)
// 		}
// 	}

// 	pyCmd := exec.Command("python", "-m", "http.server", fmt.Sprint(InspectorPort))
// 	pyCmd.Dir = folder
// 	pyCmd.Stdout = os.Stdout
// 	pyCmd.Stderr = os.Stderr
// 	if err := pyCmd.Start(); err != nil {
// 		log.Fatal("Failed to start Python server:", err)
// 	}
// 	log.Printf("Python server started at http://localhost:%d (PID %d)", InspectorPort, pyCmd.Process.Pid)

// 	go func() {
// 		if err := pyCmd.Wait(); err != nil {
// 			log.Printf("Python server exited: %v", err)
// 		}
// 	}()

// 	time.Sleep(10 * time.Second)
// 	// scrape() //TODO uncomment this

// 	log.Println("Scrape complete — Python server still running")

// 	// // restart Go server directly
// 	// go func() {
// 	// 	log.Printf("Restarting Go server on http://localhost:%d ...", APIPort)
// 	// 	if err := StartServer(DBFile, fmt.Sprintf(":%d", APIPort)); err != nil {
// 	// 		log.Fatal("Go server failed:", err)
// 	// 	}
// 	// }()

// 	select {} // keep main alive
// }
