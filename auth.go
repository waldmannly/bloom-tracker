package main

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// ─── Rate Limiting ──────────────────────────────────────────────────────────

type loginAttempt struct {
	count    int
	lastFail time.Time
}

var (
	loginAttempts = make(map[string]*loginAttempt)
	loginMu       sync.Mutex
)

const (
	maxLoginAttempts  = 5
	loginLockDuration = 15 * time.Minute
)

func checkLoginRateLimit(ip string) bool {
	loginMu.Lock()
	defer loginMu.Unlock()

	attempt, exists := loginAttempts[ip]
	if !exists {
		return true
	}
	if time.Since(attempt.lastFail) > loginLockDuration {
		delete(loginAttempts, ip)
		return true
	}
	return attempt.count < maxLoginAttempts
}

func recordFailedLogin(ip string) {
	loginMu.Lock()
	defer loginMu.Unlock()

	attempt, exists := loginAttempts[ip]
	if !exists {
		loginAttempts[ip] = &loginAttempt{count: 1, lastFail: time.Now()}
		return
	}
	attempt.count++
	attempt.lastFail = time.Now()
}

func clearFailedLogins(ip string) {
	loginMu.Lock()
	defer loginMu.Unlock()
	delete(loginAttempts, ip)
}

func getClientIP(r *http.Request) string {
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		return strings.SplitN(fwd, ",", 2)[0]
	}
	if fwd := r.Header.Get("X-Real-IP"); fwd != "" {
		return fwd
	}
	return strings.SplitN(r.RemoteAddr, ":", 2)[0]
}

// ─── CSRF Protection ────────────────────────────────────────────────────────

func generateCSRFToken(sessionToken string) string {
	mac := hmac.New(sha256.New, sessionSecret)
	mac.Write([]byte(sessionToken))
	return hex.EncodeToString(mac.Sum(nil))
}

func validateCSRFToken(r *http.Request) bool {
	cookie, err := r.Cookie("session")
	if err != nil {
		return false
	}
	token := r.FormValue("csrf_token")
	if token == "" {
		token = r.Header.Get("X-CSRF-Token")
	}
	expected := generateCSRFToken(cookie.Value)
	return hmac.Equal([]byte(token), []byte(expected))
}

func getCSRFToken(r *http.Request) string {
	cookie, err := r.Cookie("session")
	if err != nil {
		return ""
	}
	return generateCSRFToken(cookie.Value)
}

type contextKey string

const userContextKey contextKey = "user"

var sessionSecret []byte

func initSessionSecret() {
	sessionSecret = make([]byte, 32)
	if _, err := rand.Read(sessionSecret); err != nil {
		panic("failed to generate session secret: " + err.Error())
	}
	// Clean up expired sessions on startup
	cleanExpiredSessions()
}

func hashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(hash), err
}

func checkPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

func generateToken() string {
	b := make([]byte, 32)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func generateInviteCode() string {
	b := make([]byte, 3)
	rand.Read(b)
	return strings.ToUpper(hex.EncodeToString(b))
}

func createSessionCookie(w http.ResponseWriter, r *http.Request, userID int64) error {
	token := generateToken()
	expiresAt := time.Now().Add(30 * 24 * time.Hour)

	if err := insertSession(token, userID, expiresAt); err != nil {
		return err
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    token,
		Path:     "/",
		Expires:  expiresAt,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https",
	})
	return nil
}

func getSessionUser(r *http.Request) *User {
	cookie, err := r.Cookie("session")
	if err != nil {
		return nil
	}

	userID, err := getSessionUserID(cookie.Value)
	if err != nil {
		return nil
	}

	user, err := getUserByID(userID)
	if err != nil {
		return nil
	}
	return user
}

func requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := getSessionUser(r)
		if user == nil {
			setFlash(w, "error", "Please log in to continue")
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		if r.Method == http.MethodPost && !validateCSRFToken(r) {
			setFlash(w, "error", "Invalid form submission. Please try again.")
			http.Redirect(w, r, r.URL.Path, http.StatusSeeOther)
			return
		}
		ctx := context.WithValue(r.Context(), userContextKey, user)
		next(w, r.WithContext(ctx))
	}
}

func getUserFromContext(r *http.Request) *User {
	if user, ok := r.Context().Value(userContextKey).(*User); ok {
		return user
	}
	return nil
}

// ─── Auth handlers ──────────────────────────────────────────────────────────

func handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		pd := newPageData(r)
		renderTemplate(w, r, "login", pd)
		return
	}

	ip := getClientIP(r)
	if !checkLoginRateLimit(ip) {
		pd := newPageData(r)
		pd.Flash = "Too many login attempts. Please try again in 15 minutes."
		pd.FlashType = "error"
		renderTemplate(w, r, "login", pd)
		return
	}

	email := strings.TrimSpace(strings.ToLower(r.FormValue("email")))
	password := r.FormValue("password")

	if email == "" || password == "" {
		pd := newPageData(r)
		pd.Flash = "Please fill in all fields"
		pd.FlashType = "error"
		renderTemplate(w, r, "login", pd)
		return
	}

	user, err := getUserByEmail(email)
	if err != nil || !checkPassword(user.PasswordHash, password) {
		recordFailedLogin(ip)
		pd := newPageData(r)
		pd.Flash = "Invalid email or password"
		pd.FlashType = "error"
		renderTemplate(w, r, "login", pd)
		return
	}

	clearFailedLogins(ip)

	if err := createSessionCookie(w, r, user.ID); err != nil {
		pd := newPageData(r)
		pd.Flash = "Something went wrong. Please try again."
		pd.FlashType = "error"
		renderTemplate(w, r, "login", pd)
		return
	}

	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

func handleRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		pd := newPageData(r)
		renderTemplate(w, r, "register", pd)
		return
	}

	storageMode := strings.TrimSpace(strings.ToLower(r.FormValue("storage_mode")))
	if storageMode == "" || storageMode != "cloud" {
		storageMode = "local"
	}
	if storageMode == "local" {
		setFlash(w, "info", "Local-only mode enabled. Your data stays in this browser unless you export it.")
		http.Redirect(w, r, "/local", http.StatusSeeOther)
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	email := strings.TrimSpace(strings.ToLower(r.FormValue("email")))
	password := r.FormValue("password")
	confirm := r.FormValue("confirm")

	if name == "" || email == "" || password == "" {
		pd := newPageData(r)
		pd.Flash = "Please fill in all fields"
		pd.FlashType = "error"
		renderTemplate(w, r, "register", pd)
		return
	}

	if len(password) < 8 {
		pd := newPageData(r)
		pd.Flash = "Password must be at least 8 characters"
		pd.FlashType = "error"
		renderTemplate(w, r, "register", pd)
		return
	}

	if password != confirm {
		pd := newPageData(r)
		pd.Flash = "Passwords don't match"
		pd.FlashType = "error"
		renderTemplate(w, r, "register", pd)
		return
	}

	hash, err := hashPassword(password)
	if err != nil {
		pd := newPageData(r)
		pd.Flash = "Something went wrong. Please try again."
		pd.FlashType = "error"
		renderTemplate(w, r, "register", pd)
		return
	}

	_, err = createUser(email, name, hash, "owner")
	if err != nil {
		pd := newPageData(r)
		if strings.Contains(err.Error(), "UNIQUE") {
			pd.Flash = "An account with this email already exists"
		} else {
			pd.Flash = "Something went wrong. Please try again."
		}
		pd.FlashType = "error"
		renderTemplate(w, r, "register", pd)
		return
	}

	setFlash(w, "success", "Account created! Please log in.")
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func handleLogout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie("session"); err == nil {
		deleteSession(cookie.Value)
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
	})
	http.Redirect(w, r, "/", http.StatusSeeOther)
}
