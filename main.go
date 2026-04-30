package main

import (
	"bufio"
	"context"
	"embed"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

//go:embed templates/*.html
var templateFS embed.FS

//go:embed static/*
var staticFS embed.FS

func main() {
	loadEnvFile(".env")

	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "period_tracker.db"
	}

	// Database-at-rest encryption
	encKey := os.Getenv("ENCRYPTION_KEY")
	if encKey != "" {
		if len(encKey) < 8 {
			log.Fatalf("🔐 ENCRYPTION_KEY must be at least 8 characters")
		}
		if err := decryptDatabaseOnStartup(dbPath, encKey); err != nil {
			log.Fatalf("🔐 Database decryption failed: %v", err)
		}
		dbEncryptionEnabled = true
		log.Println("🔐 Database encryption enabled (AES-256-GCM, PBKDF2)")
	}

	if err := initDB(dbPath); err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer func() {
		db.Close()
		if encKey != "" {
			encryptDatabaseOnShutdown(dbPath, encKey)
		}
	}()

	initSessionSecret()
	initEmail(
		envOrDefault("SMTP_HOST", "smtp.gmail.com"),
		envOrDefault("SMTP_PORT", "587"),
		os.Getenv("SMTP_EMAIL"),
		os.Getenv("SMTP_PASS"),
		os.Getenv("EMAIL_API_URL"),
		os.Getenv("EMAIL_API_KEY"),
	)
	startWeeklyNotifier()
	if encKey != "" {
		startPeriodicEncryption(dbPath, encKey)
	}

	mux := http.NewServeMux()

	mux.Handle("/static/", http.FileServer(http.FS(staticFS)))

	// Public routes
	mux.HandleFunc("/", handleHome)
	mux.HandleFunc("/login", handleLogin)
	mux.HandleFunc("/register", handleRegister)
	mux.HandleFunc("/logout", handleLogout)
	mux.HandleFunc("/partner/join", handlePartnerJoin)
	mux.HandleFunc("/privacy", handlePrivacy)

	// Protected routes
	mux.HandleFunc("/dashboard", requireAuth(handleDashboard))
	mux.HandleFunc("/log-period", requireAuth(handleLogPeriod))
	mux.HandleFunc("/end-period", requireAuth(handleEndPeriod))
	mux.HandleFunc("/symptoms", requireAuth(handleSymptoms))
	mux.HandleFunc("/delete-symptom", requireAuth(handleDeleteSymptom))
	mux.HandleFunc("/calendar", requireAuth(handleCalendar))
	mux.HandleFunc("/settings", requireAuth(handleSettings))
	mux.HandleFunc("/partner/invite", requireAuth(handlePartnerInvite))
	mux.HandleFunc("/notifications", requireAuth(handleNotifications))
	mux.HandleFunc("/trends", requireAuth(handleTrends))
	mux.HandleFunc("/import", requireAuth(handleImport))
	mux.HandleFunc("/journal", requireAuth(handleJournal))
	mux.HandleFunc("/delete-journal", requireAuth(handleDeleteJournal))
	mux.HandleFunc("/export", requireAuth(handleExport))
	mux.HandleFunc("/delete-account", requireAuth(handleDeleteAccount))
	mux.HandleFunc("/daily-log", requireAuth(handleDailyLog))
	mux.HandleFunc("/delete-daily-log", requireAuth(handleDeleteDailyLog))
	mux.HandleFunc("/backup", requireAuth(handleBackup))
	mux.HandleFunc("/restore", requireAuth(handleRestore))
	mux.HandleFunc("/import-template", requireAuth(handleImportTemplate))
	mux.HandleFunc("/methodology", handleMethodology)

	handler := securityHeaders(mux)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	server := &http.Server{
		Addr:              ":" + port,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
		<-sigChan
		log.Println("Shutting down gracefully...")
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			log.Printf("Shutdown error: %v", err)
		}
	}()

	log.Printf("🌸 Bloom Period Tracker running on http://localhost:%s", port)
	if err := server.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatalf("Server error: %v", err)
	}
	log.Println("Server stopped")
}

func toFloat(v interface{}) float64 {
	switch n := v.(type) {
	case int:
		return float64(n)
	case int64:
		return float64(n)
	case float64:
		return n
	default:
		return 0
	}
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// loadEnvFile reads a .env file and sets any vars not already in the environment.
func loadEnvFile(path string) {
	f, err := os.Open(path)
	if err != nil {
		return // no .env file is fine
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		// Don't override existing env vars
		if os.Getenv(key) == "" {
			os.Setenv(key, val)
		}
	}
}

// PageData holds all data passed to templates
type PageData struct {
	User       *User
	IsLoggedIn bool
	IsPartner  bool
	Flash      string
	FlashType  string
	Data       interface{}
}

func newPageData(r *http.Request) *PageData {
	pd := &PageData{}
	user := getUserFromContext(r)
	if user != nil {
		pd.User = user
		pd.IsLoggedIn = true
		pd.IsPartner = user.Role == "partner"
	}
	if cookie, err := r.Cookie("flash"); err == nil {
		parts := strings.SplitN(cookie.Value, "|", 2)
		if len(parts) == 2 {
			pd.FlashType = parts[0]
			pd.Flash = parts[1]
		}
	}
	return pd
}

func setFlash(w http.ResponseWriter, flashType, message string) {
	http.SetCookie(w, &http.Cookie{
		Name:     "flash",
		Value:    flashType + "|" + message,
		Path:     "/",
		MaxAge:   10,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

func clearFlash(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:   "flash",
		Value:  "",
		Path:   "/",
		MaxAge: -1,
	})
}

func renderTemplate(w http.ResponseWriter, r *http.Request, pageName string, pd *PageData) {
	clearFlash(w)

	funcMap := template.FuncMap{
		"formatDate": func(t time.Time) string {
			return t.Format("Jan 2, 2006")
		},
		"formatDateShort": func(t time.Time) string {
			return t.Format("Mon, Jan 2")
		},
		"formatDateInput": func(t time.Time) string {
			return t.Format("2006-01-02")
		},
		"title": func(s string) string {
			if len(s) == 0 {
				return s
			}
			return strings.ToUpper(s[:1]) + s[1:]
		},
		"seq": func(n int) []int {
			s := make([]int, n)
			for i := range s {
				s[i] = i
			}
			return s
		},
		"add": func(a, b int) int { return a + b },
		"sub": func(a, b int) int { return a - b },
		"mod": func(a, b int) int { return a % b },
		"multiply": func(a, b interface{}) float64 {
			af, bf := toFloat(a), toFloat(b)
			return af * bf
		},
		"divide": func(a, b interface{}) float64 {
			af, bf := toFloat(a), toFloat(b)
			if bf == 0 {
				return 0
			}
			return af / bf
		},
		"dayName": func(i int) string {
			days := []string{"Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"}
			if i >= 0 && i < 7 {
				return days[i]
			}
			return ""
		},
		"formatTemp": func(t float64) string {
			if t == 0 {
				return "—"
			}
			return fmt.Sprintf("%.2f°C", t)
		},
		"gt": func(a, b float64) bool { return a > b },
		"tempPercent": func(t float64) float64 {
			// Map 36.0–37.5°C to 0–100%
			pct := (t - 36.0) / 1.5 * 100
			if pct < 0 {
				pct = 0
			}
			if pct > 100 {
				pct = 100
			}
			return pct
		},
	}

	tmpl, err := template.New("").Funcs(funcMap).ParseFS(
		templateFS, "templates/base.html", "templates/"+pageName+".html",
	)
	if err != nil {
		log.Printf("Template parse error (%s): %v", pageName, err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, "base", pd); err != nil {
		log.Printf("Template execute error (%s): %v", pageName, err)
	}
}
