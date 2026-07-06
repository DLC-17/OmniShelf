package main

import (
	"crypto/rand"
	"fmt"
	"log"
	"os"

	"github.com/davidlc1229/omnishelf/internal/config"
	"github.com/davidlc1229/omnishelf/internal/db"
	"github.com/davidlc1229/omnishelf/internal/models"
)

// inviteCodeLen is the length of generated invite codes.
const inviteCodeLen = 16

// inviteAlphabet deliberately omits easily confused characters (0/O, 1/I/L)
// since codes are read aloud or typed by hand.
const inviteAlphabet = "ABCDEFGHJKMNPQRSTUVWXYZ23456789"

// runInvite implements `omnishelf invite create`: generates a single-use
// registration code, stores it, and prints it (spec §2.1 step 1).
func runInvite(args []string) {
	if len(args) != 1 || args[0] != "create" {
		fmt.Fprintln(os.Stderr, "usage: omnishelf invite create")
		os.Exit(1)
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

	code, err := newInviteCode()
	if err != nil {
		log.Printf("fatal: generating invite code: %v", err)
		os.Exit(1)
	}
	if err := gdb.Create(&models.InviteCode{Code: code}).Error; err != nil {
		log.Printf("fatal: storing invite code: %v", err)
		os.Exit(1)
	}
	fmt.Println(code)
}

// newInviteCode returns a 16-character code drawn from crypto/rand.
func newInviteCode() (string, error) {
	buf := make([]byte, inviteCodeLen)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("reading random bytes: %w", err)
	}
	out := make([]byte, inviteCodeLen)
	for i, b := range buf {
		// Modulo bias is negligible for an invite code (31 symbols vs 256
		// byte values → <0.4% skew) and irrelevant to its security model.
		out[i] = inviteAlphabet[int(b)%len(inviteAlphabet)]
	}
	return string(out), nil
}
