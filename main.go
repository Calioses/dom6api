package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"os/exec"
)

const (
	DBFile        = "Data/dom6api.db"
	SqlFile       = "create_tables.sql"
	InspectorPort = 8001
	APIPort       = 8002
)

func dbcheck(filename string, sqlFile string) *sql.DB {
	log.Println("Checking database:", filename)
	db, err := sql.Open("sqlite", filename)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}

	if _, err := os.Stat(filename); os.IsNotExist(err) {
		log.Println("Database not found. Creating from SQL schema...")
		sqlBytes, err := os.ReadFile(sqlFile)
		if err != nil {
			log.Fatalf("Failed to read SQL file: %v", err)
		}

		sqlStatements := string(sqlBytes)
		_, err = db.Exec(sqlStatements)
		if err != nil {
			log.Fatalf("Failed to execute SQL file: %v", err)
		}
		log.Println("Database created successfully.")
	} else {
		log.Println("Database exists. Skipping creation.")
	}

	return db
}

func main() {
	db := dbcheck(DBFile, SqlFile)
	defer db.Close()

	// Start Go HTTP server in background
	go func() {
		log.Printf("Starting Go server on port %d...", APIPort)
		err := StartServer(DBFile, fmt.Sprintf(":%d", APIPort))

		if err != nil {
			log.Fatal("Go server failed:", err)
		}
	}()
	log.Println("Go server launch initiated.")

	// Ensure GitHub folder exists
	folder := "dom6inspector"
	repoURL := "https://github.com/larzm42/dom6inspector"
	if _, err := os.Stat(folder); os.IsNotExist(err) {
		log.Println("Folder not found. Cloning repo...")
		cmd := exec.Command("git", "clone", repoURL, folder)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			log.Fatal("Failed to clone repo:", err)
		}
		log.Println("Repo cloned successfully.")
	} else {
		log.Println("Folder exists. Skipping clone.")
	}

	// Start Python server in background
	log.Printf("Starting Python server on port %d...\n", InspectorPort)

	pyCmd := exec.Command("python", "-m", "http.server", fmt.Sprint(InspectorPort))
	pyCmd.Dir = folder
	pyCmd.Stdout = os.Stdout
	pyCmd.Stderr = os.Stderr
	if err := pyCmd.Start(); err != nil {
		log.Fatal("Failed to start Python server:", err)
	}
	log.Printf("Python server started in background on port 8001 (PID %d)\n", pyCmd.Process.Pid)

	select {} // keep main alive
}
