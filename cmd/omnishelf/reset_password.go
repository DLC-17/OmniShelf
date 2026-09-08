package main

import (
	"fmt"
	"log"
	"os"

	"golang.org/x/crypto/bcrypt"

	"github.com/davidlc1229/omnishelf/internal/config"
	"github.com/davidlc1229/omnishelf/internal/db"
	"github.com/davidlc1229/omnishelf/internal/models"
)

// runResetPassword implements `omnishelf reset-password <username> [password]`.
// If password is not provided, defaults to "admin".
// Flags the user with must_change_password = true.
func runResetPassword(args []string) {
	if len(args) < 1 || len(args) > 2 {
		fmt.Fprintln(os.Stderr, "usage: omnishelf reset-password <username> [password]")
		os.Exit(1)
	}

	username := args[0]
	newPassword := "admin"
	if len(args) == 2 {
		newPassword = args[1]
		if len(newPassword) < 8 {
			fmt.Fprintln(os.Stderr, "error: password must be at least 8 characters")
			os.Exit(1)
		}
		if len(newPassword) > 72 {
			fmt.Fprintln(os.Stderr, "error: password must be at most 72 characters")
			os.Exit(1)
		}
	}

	cfg, err := config.Load()
	if err != nil {
		log.Printf("fatal: loading configuration: %v", err)
		os.Exit(1)
	}

	gdb, err := db.Open(cfg.DataDir)
	if err != nil {
		log.Printf("fatal: opening database: %v", err)
		os.Exit(1)
	}

	var user models.User
	if err := gdb.Where("username = ?", username).First(&user).Error; err != nil {
		log.Printf("fatal: user %q not found", username)
		os.Exit(1)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), 12)
	if err != nil {
		log.Printf("fatal: hashing password: %v", err)
		os.Exit(1)
	}

	if err := gdb.Model(&user).Updates(map[string]any{
		"password_hash":        string(hash),
		"must_change_password": true,
	}).Error; err != nil {
		log.Printf("fatal: updating password: %v", err)
		os.Exit(1)
	}

	fmt.Printf("✓ Password for user %q reset successfully.\n", username)
	fmt.Printf("Temporary password: %s\n", newPassword)
	fmt.Println("User will be required to choose a new password upon next sign-in.")
}
