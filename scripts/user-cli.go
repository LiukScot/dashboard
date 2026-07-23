package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"strings"

	"golang.org/x/term"

	"github.com/LiukScot/dashboard/internal/auth"
	"github.com/LiukScot/dashboard/internal/config"
	"github.com/LiukScot/dashboard/internal/db"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: user-cli <create|list>")
		os.Exit(1)
	}

	cfg := config.Load()
	database, err := db.Open(cfg.DBPath)
	if err != nil {
		log.Fatalf("failed to open database: %v", err)
	}
	defer database.Close()

	if err := db.RunMigrations(database); err != nil {
		log.Fatalf("failed to run migrations: %v", err)
	}

	authSvc := auth.NewService(database, cfg.SessionTTL)
	reader := bufio.NewReader(os.Stdin)

	switch os.Args[1] {
	case "create":
		fmt.Print("Email: ")
		email, err := reader.ReadString('\n')
		if err != nil {
			log.Fatalf("failed to read email: %v", err)
		}
		email = strings.TrimSpace(email)

		fmt.Print("Password: ")
		var password string
		if term.IsTerminal(int(os.Stdin.Fd())) {
			passwordBytes, err := term.ReadPassword(int(os.Stdin.Fd()))
			if err != nil {
				log.Fatalf("failed to read password: %v", err)
			}
			fmt.Println()
			password = strings.TrimSpace(string(passwordBytes))
		} else {
			pw, err := reader.ReadString('\n')
			if err != nil {
				log.Fatalf("failed to read password: %v", err)
			}
			password = strings.TrimSpace(pw)
		}

		if err := authSvc.CreateUser(email, password); err != nil {
			log.Fatalf("failed to create user: %v", err)
		}
		fmt.Printf("User %s created successfully\n", email)

	case "list":
		rows, err := database.Query("SELECT id, email, created_at FROM users")
		if err != nil {
			log.Fatalf("failed to list users: %v", err)
		}
		defer rows.Close()

		fmt.Printf("%-4s %-30s %s\n", "ID", "Email", "Created")
		fmt.Println(strings.Repeat("-", 60))
		for rows.Next() {
			var id int64
			var email, createdAt string
			if err := rows.Scan(&id, &email, &createdAt); err != nil {
				log.Printf("scan user row: %v", err)
				continue
			}
			fmt.Printf("%-4d %-30s %s\n", id, email, createdAt)
		}
		if err := rows.Err(); err != nil {
			log.Fatalf("list users: %v", err)
		}

	default:
		fmt.Printf("Unknown command: %s\n", os.Args[1])
		os.Exit(1)
	}
}
