package main

import (
	"fmt"
	"log"
	"os"

	"github.com/smartinbox17-droid/canvas/backend/internal/database"
	"github.com/smartinbox17-droid/canvas/backend/internal/platform"
)

func main() {
	action := "up"
	if len(os.Args) > 1 {
		action = os.Args[1]
	}

	cfg := platform.LoadConfigFromEnv()
	db, err := database.Open(cfg)
	if err != nil {
		log.Fatalf("failed to open database: %v", err)
	}

	switch action {
	case "up":
		if err := database.MigrateUp(db); err != nil {
			log.Fatalf("migration failed: %v", err)
		}
		fmt.Println("migrations applied successfully")
	case "verify":
		if err := database.VerifyMigrations(db); err != nil {
			log.Fatalf("migration verification failed: %v", err)
		}
		fmt.Println("migrations verified successfully")
	default:
		log.Fatalf("unknown command %q (expected 'up' or 'verify')", action)
	}
}
