package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"io"
	"log"
	"os"
	"sync"
	"time"

	"golang.org/x/crypto/pbkdf2"
)

// dbEncryptionEnabled is true when ENCRYPTION_KEY is set
var dbEncryptionEnabled bool

// dbEncMu protects concurrent access during periodic encryption
var dbEncMu sync.Mutex

const (
	dbEncMagic      = "BLOMD"    // Bloom Database — distinct from backup "BLOOM"
	dbEncVersion    = byte(0x01) // format version
	dbEncHeaderSize = 6          // magic (5) + version (1)
	dbEncSaltSize   = 16
	dbEncIterations = 100_000
	dbEncKeySize    = 32 // AES-256
)

// deriveDBKey creates a 32-byte AES key from a passphrase using PBKDF2
func deriveDBKey(passphrase string, salt []byte) []byte {
	return pbkdf2.Key([]byte(passphrase), salt, dbEncIterations, dbEncKeySize, sha256.New)
}

// decryptDatabaseOnStartup decrypts the .enc file to the .db path if needed.
// If the unencrypted .db already exists (crash recovery), it's used directly.
func decryptDatabaseOnStartup(dbPath, passphrase string) error {
	encPath := dbPath + ".enc"

	// If unencrypted DB already exists, use it (crash recovery or first run)
	if _, err := os.Stat(dbPath); err == nil {
		log.Println("🔐 Existing database file found — using it directly")
		return nil
	}

	// Check for encrypted file
	encData, err := os.ReadFile(encPath)
	if err != nil {
		// No encrypted file — first run with encryption enabled
		log.Println("🔐 No encrypted database found — starting fresh")
		return nil
	}

	// Validate header
	if len(encData) < dbEncHeaderSize+dbEncSaltSize {
		return fmt.Errorf("encrypted database file too small or corrupted")
	}
	if string(encData[:5]) != dbEncMagic {
		return fmt.Errorf("invalid encrypted database file (bad magic)")
	}
	if encData[5] != dbEncVersion {
		return fmt.Errorf("unsupported encrypted database version: %d", encData[5])
	}

	// Extract salt and derive key
	salt := encData[dbEncHeaderSize : dbEncHeaderSize+dbEncSaltSize]
	key := deriveDBKey(passphrase, salt)

	// Set up AES-256-GCM
	block, err := aes.NewCipher(key)
	if err != nil {
		return fmt.Errorf("encryption setup failed: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return fmt.Errorf("encryption setup failed: %w", err)
	}

	nonceSize := gcm.NonceSize()
	payloadStart := dbEncHeaderSize + dbEncSaltSize
	if len(encData) < payloadStart+nonceSize {
		return fmt.Errorf("encrypted database file truncated")
	}

	nonce := encData[payloadStart : payloadStart+nonceSize]
	ciphertext := encData[payloadStart+nonceSize:]

	// Decrypt
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return fmt.Errorf("decryption failed — wrong encryption key or corrupted file")
	}

	// Write the unencrypted database
	if err := os.WriteFile(dbPath, plaintext, 0600); err != nil {
		return fmt.Errorf("writing decrypted database: %w", err)
	}

	log.Println("🔐 Database decrypted from encrypted storage")
	return nil
}

// encryptDatabaseFile encrypts the database file to .enc without removing the original.
// Used for periodic snapshots while the app is running.
func encryptDatabaseFile(dbPath, passphrase string) error {
	dbEncMu.Lock()
	defer dbEncMu.Unlock()

	// Checkpoint WAL so all data is in the main file
	if db != nil {
		db.Exec("PRAGMA wal_checkpoint(PASSIVE)")
	}

	plaintext, err := os.ReadFile(dbPath)
	if err != nil {
		return fmt.Errorf("reading database for encryption: %w", err)
	}

	// Generate random salt and nonce
	salt := make([]byte, dbEncSaltSize)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return fmt.Errorf("generating salt: %w", err)
	}

	key := deriveDBKey(passphrase, salt)

	block, err := aes.NewCipher(key)
	if err != nil {
		return err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return fmt.Errorf("generating nonce: %w", err)
	}

	ciphertext := gcm.Seal(nil, nonce, plaintext, nil)

	// Build encrypted file: BLOMD (5) + version (1) + salt (16) + nonce (12) + ciphertext
	encPath := dbPath + ".enc"
	tmpPath := encPath + ".tmp"

	totalSize := dbEncHeaderSize + dbEncSaltSize + len(nonce) + len(ciphertext)
	buf := make([]byte, 0, totalSize)
	buf = append(buf, []byte(dbEncMagic)...)
	buf = append(buf, dbEncVersion)
	buf = append(buf, salt...)
	buf = append(buf, nonce...)
	buf = append(buf, ciphertext...)

	// Write to temp file then atomic rename for crash safety
	if err := os.WriteFile(tmpPath, buf, 0600); err != nil {
		return fmt.Errorf("writing encrypted database: %w", err)
	}
	if err := os.Rename(tmpPath, encPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("finalizing encrypted database: %w", err)
	}

	return nil
}

// encryptDatabaseOnShutdown does a full WAL checkpoint, encrypts, and removes unencrypted files.
func encryptDatabaseOnShutdown(dbPath, passphrase string) {
	// Full WAL checkpoint before close
	if db != nil {
		db.Exec("PRAGMA wal_checkpoint(TRUNCATE)")
	}

	if err := encryptDatabaseFile(dbPath, passphrase); err != nil {
		log.Printf("🔐 WARNING: Failed to encrypt database on shutdown: %v", err)
		return
	}

	// Remove unencrypted files
	os.Remove(dbPath)
	os.Remove(dbPath + "-shm")
	os.Remove(dbPath + "-wal")

	log.Println("🔐 Database encrypted and unencrypted files removed")
}

// startPeriodicEncryption runs a background goroutine that encrypts the database
// every 5 minutes for crash resilience. If the app crashes, the .enc file is at
// most 5 minutes old.
func startPeriodicEncryption(dbPath, passphrase string) {
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			if err := encryptDatabaseFile(dbPath, passphrase); err != nil {
				log.Printf("🔐 Periodic encryption snapshot failed: %v", err)
			}
		}
	}()
	log.Println("🔐 Periodic encryption snapshots enabled (every 5 minutes)")
}
