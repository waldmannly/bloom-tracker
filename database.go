package main

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

var db *sql.DB

// User represents a registered user (owner or partner)
type User struct {
	ID            int64
	Email         string
	Name          string
	PasswordHash  string
	Role          string // "owner" or "partner"
	PartnerOf     *int64 // owner's user ID (if role is "partner")
	PartnerCode   string
	CycleLength   int
	PeriodLength  int
	PartnerNotify bool
	ShowFertility bool   // whether to display fertile window/ovulation info
	Pronouns      string // "she/her", "he/him", "they/them"
	Theme         string // "bloom" or "ocean"
	CreatedAt     string
}

// JournalEntry represents a daily journal entry
type JournalEntry struct {
	ID        int64
	UserID    int64
	Date      time.Time
	MoodEmoji string
	Title     string
	Content   string
	CreatedAt string
}

// DailyReading holds basal body temperature and cervical mucus for one day
type DailyReading struct {
	ID            int64
	UserID        int64
	Date          time.Time
	BasalTemp     float64 // e.g. 36.5 °C
	CervicalMucus string  // "dry", "sticky", "creamy", "egg-white", ""
	Notes         string
	SleepQuality  int // 1-5 scale, 0 = not recorded
	StressLevel   int // 1-5 scale, 0 = not recorded
	EnergyLevel   int // 1-5 scale, 0 = not recorded
	CreatedAt     string
}

// Period represents a logged menstrual period
type Period struct {
	ID        int64
	UserID    int64
	StartDate time.Time
	EndDate   *time.Time
}

// Symptom represents a logged symptom
type Symptom struct {
	ID       int64
	UserID   int64
	Date     time.Time
	Category string
	Name     string
	Severity int
	Notes    string
}

func initDB(path string) error {
	var err error
	db, err = sql.Open("sqlite", path)
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}

	// Performance and safety settings
	pragmas := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA foreign_keys=ON",
		"PRAGMA busy_timeout=5000",
	}
	for _, p := range pragmas {
		if _, err := db.Exec(p); err != nil {
			return fmt.Errorf("setting pragma: %w", err)
		}
	}

	return createSchema()
}

func createSchema() error {
	schema := `
	CREATE TABLE IF NOT EXISTS users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		email TEXT UNIQUE NOT NULL COLLATE NOCASE,
		name TEXT NOT NULL,
		password_hash TEXT NOT NULL,
		role TEXT NOT NULL DEFAULT 'owner',
		partner_of INTEGER REFERENCES users(id),
		partner_code TEXT,
		cycle_length INTEGER NOT NULL DEFAULT 28,
		period_length INTEGER NOT NULL DEFAULT 5,
		partner_notify INTEGER NOT NULL DEFAULT 0,
		last_notified_phase TEXT DEFAULT '',
		show_fertility INTEGER NOT NULL DEFAULT 1,
		pronouns TEXT NOT NULL DEFAULT 'she/her',
		theme TEXT NOT NULL DEFAULT 'bloom',
		created_at TEXT DEFAULT (datetime('now'))
	);

	CREATE TABLE IF NOT EXISTS sessions (
		token TEXT PRIMARY KEY,
		user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		created_at TEXT DEFAULT (datetime('now')),
		expires_at TEXT NOT NULL
	);

	CREATE TABLE IF NOT EXISTS periods (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		start_date TEXT NOT NULL,
		end_date TEXT,
		created_at TEXT DEFAULT (datetime('now'))
	);

	CREATE TABLE IF NOT EXISTS symptoms (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		date TEXT NOT NULL,
		category TEXT NOT NULL,
		symptom TEXT NOT NULL,
		severity INTEGER NOT NULL DEFAULT 1 CHECK(severity BETWEEN 1 AND 5),
		notes TEXT DEFAULT '',
		created_at TEXT DEFAULT (datetime('now'))
	);

	CREATE TABLE IF NOT EXISTS notifications (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		channel TEXT NOT NULL,
		phase TEXT NOT NULL,
		sent_at TEXT DEFAULT (datetime('now'))
	);

	CREATE TABLE IF NOT EXISTS journal_entries (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		date TEXT NOT NULL,
		mood_emoji TEXT DEFAULT '',
		title TEXT DEFAULT '',
		content TEXT DEFAULT '',
		created_at TEXT DEFAULT (datetime('now')),
		UNIQUE(user_id, date)
	);

	CREATE TABLE IF NOT EXISTS daily_readings (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		date TEXT NOT NULL,
		basal_temp REAL,
		cervical_mucus TEXT DEFAULT '',
		notes TEXT DEFAULT '',
		created_at TEXT DEFAULT (datetime('now')),
		UNIQUE(user_id, date)
	);

	CREATE INDEX IF NOT EXISTS idx_periods_user ON periods(user_id, start_date DESC);
	CREATE INDEX IF NOT EXISTS idx_symptoms_user ON symptoms(user_id, date DESC);
	CREATE INDEX IF NOT EXISTS idx_sessions_expires ON sessions(expires_at);
	CREATE INDEX IF NOT EXISTS idx_notifications_user ON notifications(user_id, sent_at DESC);
	CREATE INDEX IF NOT EXISTS idx_journal_user ON journal_entries(user_id, date DESC);
	CREATE INDEX IF NOT EXISTS idx_daily_readings_user ON daily_readings(user_id, date DESC);

	CREATE TABLE IF NOT EXISTS phase_preferences (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		phase TEXT NOT NULL,
		preferences TEXT DEFAULT '',
		UNIQUE(user_id, phase)
	);
	`
	_, err := db.Exec(schema)
	if err != nil {
		return err
	}

	// Migrations for pre-existing databases
	migrations := []string{
		"ALTER TABLE users ADD COLUMN partner_notify INTEGER NOT NULL DEFAULT 0",
		"ALTER TABLE users ADD COLUMN last_notified_phase TEXT DEFAULT ''",
		"ALTER TABLE users ADD COLUMN show_fertility INTEGER NOT NULL DEFAULT 1",
		"ALTER TABLE users ADD COLUMN pronouns TEXT NOT NULL DEFAULT 'she/her'",
		"ALTER TABLE daily_readings ADD COLUMN sleep_quality INTEGER",
		"ALTER TABLE daily_readings ADD COLUMN stress_level INTEGER",
		"ALTER TABLE daily_readings ADD COLUMN energy_level INTEGER",
		"ALTER TABLE users ADD COLUMN theme TEXT NOT NULL DEFAULT 'bloom'",
	}
	for _, m := range migrations {
		db.Exec(m) // ignore errors (column already exists)
	}
	return nil
}

// ─── User operations ────────────────────────────────────────────────────────

func createUser(email, name, passwordHash, role string) (int64, error) {
	res, err := db.Exec(
		"INSERT INTO users (email, name, password_hash, role) VALUES (?, ?, ?, ?)",
		email, name, passwordHash, role,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func getUserByEmail(email string) (*User, error) {
	return scanUser(db.QueryRow(
		"SELECT id, email, name, password_hash, role, partner_of, partner_code, cycle_length, period_length, partner_notify, show_fertility, pronouns, theme FROM users WHERE email = ?",
		email,
	))
}

func getUserByID(id int64) (*User, error) {
	return scanUser(db.QueryRow(
		"SELECT id, email, name, password_hash, role, partner_of, partner_code, cycle_length, period_length, partner_notify, show_fertility, pronouns, theme FROM users WHERE id = ?",
		id,
	))
}

func scanUser(row *sql.Row) (*User, error) {
	var u User
	var partnerOf sql.NullInt64
	var partnerCode sql.NullString
	var pronouns sql.NullString
	var theme sql.NullString
	err := row.Scan(&u.ID, &u.Email, &u.Name, &u.PasswordHash, &u.Role,
		&partnerOf, &partnerCode, &u.CycleLength, &u.PeriodLength, &u.PartnerNotify, &u.ShowFertility, &pronouns, &theme)
	if err != nil {
		return nil, err
	}
	if partnerOf.Valid {
		u.PartnerOf = &partnerOf.Int64
	}
	if partnerCode.Valid {
		u.PartnerCode = partnerCode.String
	}
	u.Pronouns = "she/her"
	if pronouns.Valid && pronouns.String != "" {
		u.Pronouns = pronouns.String
	}
	u.Theme = "bloom"
	if theme.Valid && theme.String != "" {
		u.Theme = theme.String
	}
	return &u, nil
}

func updateUserSettings(id int64, cycleLength, periodLength int, showFertility bool, pronouns, theme string) error {
	_, err := db.Exec(
		"UPDATE users SET cycle_length = ?, period_length = ?, show_fertility = ?, pronouns = ?, theme = ? WHERE id = ?",
		cycleLength, periodLength, showFertility, pronouns, theme, id,
	)
	return err
}

func getPhasePreferences(userID int64) map[string]string {
	result := map[string]string{}
	rows, err := db.Query("SELECT phase, preferences FROM phase_preferences WHERE user_id = ?", userID)
	if err != nil {
		return result
	}
	defer rows.Close()
	for rows.Next() {
		var phase, prefs string
		rows.Scan(&phase, &prefs)
		result[phase] = prefs
	}
	return result
}

func savePhasePreference(userID int64, phase, preferences string) error {
	_, err := db.Exec(`
		INSERT INTO phase_preferences (user_id, phase, preferences)
		VALUES (?, ?, ?)
		ON CONFLICT(user_id, phase) DO UPDATE SET preferences=excluded.preferences`,
		userID, phase, preferences,
	)
	return err
}

func setPartnerCode(userID int64, code string) error {
	_, err := db.Exec("UPDATE users SET partner_code = ? WHERE id = ?", code, userID)
	return err
}

func getUserByPartnerCode(code string) (*User, error) {
	return scanUser(db.QueryRow(
		"SELECT id, email, name, password_hash, role, partner_of, partner_code, cycle_length, period_length, partner_notify, show_fertility, pronouns, theme FROM users WHERE partner_code = ? AND role = 'owner'",
		code,
	))
}

func linkPartner(partnerID, ownerID int64) error {
	_, err := db.Exec("UPDATE users SET partner_of = ? WHERE id = ?", ownerID, partnerID)
	return err
}

func getPartnerForOwner(ownerID int64) (*User, error) {
	return scanUser(db.QueryRow(
		"SELECT id, email, name, password_hash, role, partner_of, partner_code, cycle_length, period_length, partner_notify, show_fertility, pronouns, theme FROM users WHERE partner_of = ?",
		ownerID,
	))
}

// ─── Session operations ─────────────────────────────────────────────────────

func insertSession(token string, userID int64, expiresAt time.Time) error {
	_, err := db.Exec(
		"INSERT INTO sessions (token, user_id, expires_at) VALUES (?, ?, ?)",
		token, userID, expiresAt.Format("2006-01-02 15:04:05"),
	)
	return err
}

func getSessionUserID(token string) (int64, error) {
	var userID int64
	err := db.QueryRow(
		"SELECT user_id FROM sessions WHERE token = ? AND expires_at > datetime('now')",
		token,
	).Scan(&userID)
	return userID, err
}

func deleteSession(token string) {
	db.Exec("DELETE FROM sessions WHERE token = ?", token)
}

func cleanExpiredSessions() {
	db.Exec("DELETE FROM sessions WHERE expires_at < datetime('now')")
}

// ─── Period operations ──────────────────────────────────────────────────────

func logPeriodStart(userID int64, startDate time.Time) error {
	_, err := db.Exec(
		"INSERT INTO periods (user_id, start_date) VALUES (?, ?)",
		userID, startDate.Format("2006-01-02"),
	)
	return err
}

func endPeriod(periodID int64, endDate time.Time) error {
	_, err := db.Exec(
		"UPDATE periods SET end_date = ? WHERE id = ?",
		endDate.Format("2006-01-02"), periodID,
	)
	return err
}

func updatePeriod(periodID int64, startDate time.Time, endDate *time.Time) error {
	if endDate != nil {
		_, err := db.Exec(
			"UPDATE periods SET start_date = ?, end_date = ? WHERE id = ?",
			startDate.Format("2006-01-02"), endDate.Format("2006-01-02"), periodID,
		)
		return err
	}
	_, err := db.Exec(
		"UPDATE periods SET start_date = ?, end_date = NULL WHERE id = ?",
		startDate.Format("2006-01-02"), periodID,
	)
	return err
}

func deletePeriod(periodID, userID int64) error {
	_, err := db.Exec("DELETE FROM periods WHERE id = ? AND user_id = ?", periodID, userID)
	return err
}

func getPeriodByID(periodID, userID int64) (*Period, error) {
	var p Period
	var startStr string
	var endStr sql.NullString
	err := db.QueryRow(
		"SELECT id, user_id, start_date, end_date FROM periods WHERE id = ? AND user_id = ?",
		periodID, userID,
	).Scan(&p.ID, &p.UserID, &startStr, &endStr)
	if err != nil {
		return nil, err
	}
	p.StartDate = parseDate(startStr)
	if endStr.Valid {
		t := parseDate(endStr.String)
		p.EndDate = &t
	}
	return &p, nil
}

func getActivePeriod(userID int64) (*Period, error) {
	var p Period
	var startStr string
	err := db.QueryRow(
		"SELECT id, user_id, start_date FROM periods WHERE user_id = ? AND end_date IS NULL ORDER BY start_date DESC LIMIT 1",
		userID,
	).Scan(&p.ID, &p.UserID, &startStr)
	if err != nil {
		return nil, err
	}
	p.StartDate = parseDate(startStr)
	return &p, nil
}

func getLastPeriod(userID int64) (*Period, error) {
	var p Period
	var startStr string
	var endStr sql.NullString
	err := db.QueryRow(
		"SELECT id, user_id, start_date, end_date FROM periods WHERE user_id = ? ORDER BY start_date DESC LIMIT 1",
		userID,
	).Scan(&p.ID, &p.UserID, &startStr, &endStr)
	if err != nil {
		return nil, err
	}
	p.StartDate = parseDate(startStr)
	if endStr.Valid {
		t := parseDate(endStr.String)
		p.EndDate = &t
	}
	return &p, nil
}

func getPeriods(userID int64, limit int) ([]Period, error) {
	rows, err := db.Query(
		"SELECT id, user_id, start_date, end_date FROM periods WHERE user_id = ? ORDER BY start_date DESC LIMIT ?",
		userID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanPeriods(rows)
}

func getPeriodsInRange(userID int64, start, end time.Time) ([]Period, error) {
	rows, err := db.Query(
		"SELECT id, user_id, start_date, end_date FROM periods WHERE user_id = ? AND start_date >= ? AND start_date <= ? ORDER BY start_date",
		userID, start.Format("2006-01-02"), end.Format("2006-01-02"),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanPeriods(rows)
}

func scanPeriods(rows *sql.Rows) ([]Period, error) {
	var periods []Period
	for rows.Next() {
		var p Period
		var startStr string
		var endStr sql.NullString
		if err := rows.Scan(&p.ID, &p.UserID, &startStr, &endStr); err != nil {
			return nil, err
		}
		p.StartDate = parseDate(startStr)
		if endStr.Valid {
			t := parseDate(endStr.String)
			p.EndDate = &t
		}
		periods = append(periods, p)
	}
	return periods, rows.Err()
}

// ─── Symptom operations ─────────────────────────────────────────────────────

func logSymptom(userID int64, date time.Time, category, symptom string, severity int, notes string) error {
	_, err := db.Exec(
		"INSERT INTO symptoms (user_id, date, category, symptom, severity, notes) VALUES (?, ?, ?, ?, ?, ?)",
		userID, date.Format("2006-01-02"), category, symptom, severity, notes,
	)
	return err
}

func getSymptomsForDate(userID int64, date time.Time) ([]Symptom, error) {
	rows, err := db.Query(
		"SELECT id, user_id, date, category, symptom, severity, notes FROM symptoms WHERE user_id = ? AND date = ? ORDER BY created_at DESC",
		userID, date.Format("2006-01-02"),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSymptoms(rows)
}

func getRecentSymptoms(userID int64, limit int) ([]Symptom, error) {
	rows, err := db.Query(
		"SELECT id, user_id, date, category, symptom, severity, notes FROM symptoms WHERE user_id = ? ORDER BY date DESC, created_at DESC LIMIT ?",
		userID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSymptoms(rows)
}

func getSymptomsInRange(userID int64, start, end time.Time) ([]Symptom, error) {
	rows, err := db.Query(
		"SELECT id, user_id, date, category, symptom, severity, notes FROM symptoms WHERE user_id = ? AND date >= ? AND date <= ? ORDER BY date",
		userID, start.Format("2006-01-02"), end.Format("2006-01-02"),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSymptoms(rows)
}

func deleteSymptom(symptomID, userID int64) error {
	_, err := db.Exec("DELETE FROM symptoms WHERE id = ? AND user_id = ?", symptomID, userID)
	return err
}

func scanSymptoms(rows *sql.Rows) ([]Symptom, error) {
	var symptoms []Symptom
	for rows.Next() {
		var s Symptom
		var dateStr string
		var notes sql.NullString
		if err := rows.Scan(&s.ID, &s.UserID, &dateStr, &s.Category, &s.Name, &s.Severity, &notes); err != nil {
			return nil, err
		}
		s.Date = parseDate(dateStr)
		if notes.Valid {
			s.Notes = notes.String
		}
		symptoms = append(symptoms, s)
	}
	return symptoms, rows.Err()
}

// ─── Trend / Analytics operations ───────────────────────────────────────────

// CycleTrend holds data for one cycle (period-to-period)
type CycleTrend struct {
	StartDate   time.Time
	CycleLength int
	PeriodDays  int
	MonthLabel  string
}

// SymptomTrend holds aggregated symptom data
type SymptomTrend struct {
	Name     string
	Category string
	Count    int
	AvgSev   float64
}

// MonthSummary holds one month's aggregated data
type MonthSummary struct {
	Month         string
	Year          int
	MonthNum      int
	PeriodCount   int
	AvgCycleLen   float64
	AvgPeriodDays float64
	SymptomCount  int
	TopSymptom    string
}

func getCycleTrends(userID int64, limit int) ([]CycleTrend, error) {
	periods, err := getPeriods(userID, limit+1)
	if err != nil || len(periods) < 2 {
		return nil, err
	}

	var trends []CycleTrend
	// periods are DESC, so we iterate from oldest to newest
	for i := len(periods) - 1; i > 0; i-- {
		cycleLen := daysBetween(periods[i].StartDate, periods[i-1].StartDate)
		periodDays := 0
		if periods[i].EndDate != nil {
			periodDays = daysBetween(periods[i].StartDate, *periods[i].EndDate) + 1
		}
		if cycleLen > 0 && cycleLen < 60 {
			trends = append(trends, CycleTrend{
				StartDate:   periods[i].StartDate,
				CycleLength: cycleLen,
				PeriodDays:  periodDays,
				MonthLabel:  periods[i].StartDate.Format("Jan '06"),
			})
		}
	}
	return trends, nil
}

func getTopSymptoms(userID int64, limit int) ([]SymptomTrend, error) {
	rows, err := db.Query(`
		SELECT symptom, category, COUNT(*) as cnt, ROUND(AVG(severity), 1) as avg_sev
		FROM symptoms WHERE user_id = ?
		GROUP BY symptom, category
		ORDER BY cnt DESC LIMIT ?`,
		userID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var trends []SymptomTrend
	for rows.Next() {
		var t SymptomTrend
		if err := rows.Scan(&t.Name, &t.Category, &t.Count, &t.AvgSev); err != nil {
			return nil, err
		}
		trends = append(trends, t)
	}
	return trends, rows.Err()
}

func getMonthSummaries(userID int64, monthCount int) ([]MonthSummary, error) {
	now := time.Now()
	var summaries []MonthSummary

	for i := monthCount - 1; i >= 0; i-- {
		t := now.AddDate(0, -i, 0)
		y, m := t.Year(), int(t.Month())
		monthStart := time.Date(y, t.Month(), 1, 0, 0, 0, 0, time.Local)
		monthEnd := monthStart.AddDate(0, 1, -1)

		periods, _ := getPeriodsInRange(userID, monthStart.AddDate(0, 0, -45), monthEnd)
		symptoms, _ := getSymptomsInRange(userID, monthStart, monthEnd)

		pCount := 0
		for _, p := range periods {
			if !p.StartDate.Before(monthStart) && !p.StartDate.After(monthEnd) {
				pCount++
			}
		}

		// Top symptom this month
		topSym := ""
		symCounts := map[string]int{}
		for _, s := range symptoms {
			symCounts[s.Name]++
		}
		maxC := 0
		for name, c := range symCounts {
			if c > maxC {
				maxC = c
				topSym = name
			}
		}

		summaries = append(summaries, MonthSummary{
			Month:        monthStart.Format("Jan"),
			Year:         y,
			MonthNum:     m,
			PeriodCount:  pCount,
			SymptomCount: len(symptoms),
			TopSymptom:   topSym,
		})
	}
	return summaries, nil
}

func getTotalPeriodCount(userID int64) int {
	var count int
	db.QueryRow("SELECT COUNT(*) FROM periods WHERE user_id = ?", userID).Scan(&count)
	return count
}

func getTotalSymptomCount(userID int64) int {
	var count int
	db.QueryRow("SELECT COUNT(*) FROM symptoms WHERE user_id = ?", userID).Scan(&count)
	return count
}

func getAverageCycleLength(userID int64) float64 {
	trends, err := getCycleTrends(userID, 50)
	if err != nil || len(trends) == 0 {
		return 0
	}
	total := 0
	for _, t := range trends {
		total += t.CycleLength
	}
	return float64(total) / float64(len(trends))
}

// ─── Helpers ────────────────────────────────────────────────────────────────

func parseDate(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		t, err = time.Parse("2006-01-02 15:04:05", s)
		if err != nil {
			return time.Time{}
		}
	}
	return t
}

// ─── Notification operations ────────────────────────────────────────────────

// NotifyPair links an owner's cycle data to their partner's email for daily notifications
type NotifyPair struct {
	OwnerID      int64
	OwnerName    string
	CycleLength  int
	PeriodLength int
	PartnerID    int64
	PartnerName  string
	PartnerEmail string
}

func getOwnersWithNotifyPartners() ([]NotifyPair, error) {
	rows, err := db.Query(`
		SELECT o.id, o.name, o.cycle_length, o.period_length,
		       p.id, p.name, p.email
		FROM users o
		JOIN users p ON p.partner_of = o.id
		WHERE o.role = 'owner'
		  AND o.partner_notify = 1
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var pairs []NotifyPair
	for rows.Next() {
		var np NotifyPair
		if err := rows.Scan(&np.OwnerID, &np.OwnerName, &np.CycleLength, &np.PeriodLength,
			&np.PartnerID, &np.PartnerName, &np.PartnerEmail); err != nil {
			return nil, err
		}
		pairs = append(pairs, np)
	}
	return pairs, rows.Err()
}

func wasNotificationSentToday(userID int64, today time.Time) bool {
	var count int
	err := db.QueryRow(
		"SELECT COUNT(*) FROM notifications WHERE user_id = ? AND date(sent_at) = ?",
		userID, today.Format("2006-01-02"),
	).Scan(&count)
	return err == nil && count > 0
}

func logNotification(userID int64, channel, phase string) {
	db.Exec(
		"INSERT INTO notifications (user_id, channel, phase) VALUES (?, ?, ?)",
		userID, channel, phase,
	)
}

// ─── Journal operations ─────────────────────────────────────────────────────

func saveJournalEntry(userID int64, date time.Time, moodEmoji, title, content string) error {
	// Upsert: one entry per day
	_, err := db.Exec(`
		INSERT INTO journal_entries (user_id, date, mood_emoji, title, content)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(user_id, date) DO UPDATE SET mood_emoji=excluded.mood_emoji, title=excluded.title, content=excluded.content`,
		userID, date.Format("2006-01-02"), moodEmoji, title, content,
	)
	return err
}

func getJournalEntries(userID int64, limit int) ([]JournalEntry, error) {
	rows, err := db.Query(
		"SELECT id, user_id, date, mood_emoji, title, content, created_at FROM journal_entries WHERE user_id = ? ORDER BY date DESC LIMIT ?",
		userID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []JournalEntry
	for rows.Next() {
		var e JournalEntry
		var dateStr string
		var moodEmoji, title, content sql.NullString
		var createdAt sql.NullString
		if err := rows.Scan(&e.ID, &e.UserID, &dateStr, &moodEmoji, &title, &content, &createdAt); err != nil {
			return nil, err
		}
		e.Date = parseDate(dateStr)
		if moodEmoji.Valid {
			e.MoodEmoji = moodEmoji.String
		}
		if title.Valid {
			e.Title = title.String
		}
		if content.Valid {
			e.Content = content.String
		}
		if createdAt.Valid {
			e.CreatedAt = createdAt.String
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

func deleteJournalEntry(entryID, userID int64) error {
	_, err := db.Exec("DELETE FROM journal_entries WHERE id = ? AND user_id = ?", entryID, userID)
	return err
}

// ─── Account deletion ───────────────────────────────────────────────────────

func deleteUserAccount(userID int64) error {
	db.Exec("UPDATE users SET partner_of = NULL WHERE partner_of = ?", userID)
	queries := []string{
		"DELETE FROM phase_preferences WHERE user_id = ?",
		"DELETE FROM daily_readings WHERE user_id = ?",
		"DELETE FROM journal_entries WHERE user_id = ?",
		"DELETE FROM notifications WHERE user_id = ?",
		"DELETE FROM symptoms WHERE user_id = ?",
		"DELETE FROM periods WHERE user_id = ?",
		"DELETE FROM sessions WHERE user_id = ?",
	}
	for _, q := range queries {
		if _, err := db.Exec(q, userID); err != nil {
			return err
		}
	}
	_, err := db.Exec("DELETE FROM users WHERE id = ?", userID)
	return err
}

// ─── Data export ────────────────────────────────────────────────────────────

func getAllPeriods(userID int64) ([]Period, error) {
	rows, err := db.Query(
		"SELECT id, user_id, start_date, end_date FROM periods WHERE user_id = ? ORDER BY start_date",
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanPeriods(rows)
}

func getAllSymptoms(userID int64) ([]Symptom, error) {
	rows, err := db.Query(
		"SELECT id, user_id, date, category, symptom, severity, notes FROM symptoms WHERE user_id = ? ORDER BY date",
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSymptoms(rows)
}

func getAllJournalEntries(userID int64) ([]JournalEntry, error) {
	return getJournalEntries(userID, 100000)
}

// ─── Symptom-phase correlation ──────────────────────────────────────────────

// PhaseSymptomCorrelation holds symptom frequency for a specific cycle phase
type PhaseSymptomCorrelation struct {
	Phase    string
	Symptom  string
	Category string
	Count    int
	AvgSev   float64
}

func getSymptomPhaseCorrelations(userID int64, cycleLength, periodLength int) ([]PhaseSymptomCorrelation, error) {
	// Get all symptoms and periods, then compute which phase each symptom fell in
	allPeriods, err := getAllPeriods(userID)
	if err != nil || len(allPeriods) == 0 {
		return nil, err
	}
	allSymptoms, err := getAllSymptoms(userID)
	if err != nil || len(allSymptoms) == 0 {
		return nil, err
	}

	type phaseKey struct {
		Phase, Symptom, Category string
	}
	counts := map[phaseKey][]int{}

	for _, s := range allSymptoms {
		// Find which period cycle this symptom belongs to
		var bestPeriod *Period
		for i := range allPeriods {
			if !allPeriods[i].StartDate.After(s.Date) {
				bestPeriod = &allPeriods[i]
			}
		}
		if bestPeriod == nil {
			continue
		}
		info := calculateCycleInfo(bestPeriod.StartDate, cycleLength, periodLength, s.Date)
		pk := phaseKey{Phase: info.Phase, Symptom: s.Name, Category: s.Category}
		counts[pk] = append(counts[pk], s.Severity)
	}

	var results []PhaseSymptomCorrelation
	for pk, sevs := range counts {
		total := 0
		for _, v := range sevs {
			total += v
		}
		results = append(results, PhaseSymptomCorrelation{
			Phase:    pk.Phase,
			Symptom:  pk.Symptom,
			Category: pk.Category,
			Count:    len(sevs),
			AvgSev:   float64(total) / float64(len(sevs)),
		})
	}
	return results, nil
}

// ─── Smart alerts ───────────────────────────────────────────────────────────

// SmartAlert represents an insight or warning about cycle patterns
type SmartAlert struct {
	Icon    string
	Title   string
	Message string
	Type    string // "info", "warning", "success"
}

func generateSmartAlerts(userID int64, cycleLength, periodLength int) []SmartAlert {
	var alerts []SmartAlert

	trends, err := getCycleTrends(userID, 12)
	if err != nil || len(trends) < 2 {
		return alerts
	}

	// Check cycle regularity
	var lengths []int
	total := 0
	for _, t := range trends {
		lengths = append(lengths, t.CycleLength)
		total += t.CycleLength
	}
	avg := float64(total) / float64(len(lengths))
	maxDiff := 0.0
	for _, l := range lengths {
		diff := float64(l) - avg
		if diff < 0 {
			diff = -diff
		}
		if diff > maxDiff {
			maxDiff = diff
		}
	}

	if maxDiff > 7 {
		alerts = append(alerts, SmartAlert{
			Icon:    "⚠️",
			Title:   "Irregular Cycles Detected",
			Message: fmt.Sprintf("Your cycles have varied by up to %.0f days. If this is unusual for you, consider mentioning it to your healthcare provider.", maxDiff),
			Type:    "warning",
		})
	} else if len(trends) >= 3 && maxDiff <= 3 {
		alerts = append(alerts, SmartAlert{
			Icon:    "✅",
			Title:   "Regular Cycles",
			Message: fmt.Sprintf("Your cycles have been very consistent (avg %.1f days). Keep tracking to maintain this insight!", avg),
			Type:    "success",
		})
	}

	// Check for late period
	lastPeriod, err := getLastPeriod(userID)
	if err == nil {
		daysSince := daysBetween(lastPeriod.StartDate, time.Now())
		if daysSince > cycleLength+5 {
			late := daysSince - cycleLength
			alerts = append(alerts, SmartAlert{
				Icon:    "🔔",
				Title:   "Period May Be Late",
				Message: fmt.Sprintf("It's been %d days since your last period (%d days past your usual %d-day cycle). Stress, lifestyle changes, or other factors can cause this.", daysSince, late, cycleLength),
				Type:    "warning",
			})
		}
	}

	// Check recent symptom severity spike
	recent, _ := getRecentSymptoms(userID, 20)
	if len(recent) >= 5 {
		severeCount := 0
		for _, s := range recent[:5] {
			if s.Severity >= 4 {
				severeCount++
			}
		}
		if severeCount >= 3 {
			alerts = append(alerts, SmartAlert{
				Icon:    "💛",
				Title:   "High Symptom Severity",
				Message: "You've logged several severe symptoms recently. Remember to take care of yourself — rest, hydrate, and don't hesitate to seek support.",
				Type:    "info",
			})
		}
	}

	return alerts
}

// ─── Daily readings (BBT + cervical mucus) ──────────────────────────────────

func saveDailyReading(userID int64, date time.Time, basalTemp float64, cervicalMucus, notes string, sleepQuality, stressLevel, energyLevel int) error {
	_, err := db.Exec(`
		INSERT INTO daily_readings (user_id, date, basal_temp, cervical_mucus, notes, sleep_quality, stress_level, energy_level)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(user_id, date) DO UPDATE SET basal_temp=excluded.basal_temp, cervical_mucus=excluded.cervical_mucus, notes=excluded.notes, sleep_quality=excluded.sleep_quality, stress_level=excluded.stress_level, energy_level=excluded.energy_level`,
		userID, date.Format("2006-01-02"), basalTemp, cervicalMucus, notes, sleepQuality, stressLevel, energyLevel,
	)
	return err
}

func getDailyReadings(userID int64, limit int) ([]DailyReading, error) {
	rows, err := db.Query(
		"SELECT id, user_id, date, basal_temp, cervical_mucus, notes, created_at, sleep_quality, stress_level, energy_level FROM daily_readings WHERE user_id = ? ORDER BY date DESC LIMIT ?",
		userID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanDailyReadings(rows)
}

func getDailyReadingsInRange(userID int64, start, end time.Time) ([]DailyReading, error) {
	rows, err := db.Query(
		"SELECT id, user_id, date, basal_temp, cervical_mucus, notes, created_at, sleep_quality, stress_level, energy_level FROM daily_readings WHERE user_id = ? AND date >= ? AND date <= ? ORDER BY date",
		userID, start.Format("2006-01-02"), end.Format("2006-01-02"),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanDailyReadings(rows)
}

func getAllDailyReadings(userID int64) ([]DailyReading, error) {
	return getDailyReadings(userID, 100000)
}

func deleteDailyReading(readingID, userID int64) error {
	_, err := db.Exec("DELETE FROM daily_readings WHERE id = ? AND user_id = ?", readingID, userID)
	return err
}

// ─── Year-at-a-Glance Calendar ──────────────────────────────────────────────

type YearMonth struct {
	Name  string
	Year  int
	Weeks [][]YearDay
}

type YearDay struct {
	Day         int
	InMonth     bool
	IsPeriod    bool
	IsPredicted bool
	IsToday     bool
	Phase       string
}

func generateYearCalendar(userID int64, cycleLength, periodLength int, lastPeriodStart *time.Time) []YearMonth {
	now := time.Now()
	year := now.Year()
	today := midnight(now)

	// Load all periods for this year
	yearStart := time.Date(year, 1, 1, 0, 0, 0, 0, time.Local)
	yearEnd := time.Date(year, 12, 31, 0, 0, 0, 0, time.Local)
	periods, _ := getPeriodsInRange(userID, yearStart.AddDate(0, 0, -40), yearEnd)

	periodDays := make(map[string]bool)
	for _, p := range periods {
		endDate := p.StartDate.AddDate(0, 0, periodLength-1)
		if p.EndDate != nil {
			endDate = *p.EndDate
		}
		for d := p.StartDate; !d.After(endDate); d = d.AddDate(0, 0, 1) {
			periodDays[d.Format("2006-01-02")] = true
		}
	}

	var months []YearMonth
	for m := 1; m <= 12; m++ {
		firstOfMonth := time.Date(year, time.Month(m), 1, 0, 0, 0, 0, time.Local)
		lastOfMonth := firstOfMonth.AddDate(0, 1, -1)

		gridStart := firstOfMonth.AddDate(0, 0, -int(firstOfMonth.Weekday()))
		gridEnd := lastOfMonth.AddDate(0, 0, 6-int(lastOfMonth.Weekday()))

		var weeks [][]YearDay
		current := gridStart
		for !current.After(gridEnd) {
			var week []YearDay
			for i := 0; i < 7; i++ {
				key := current.Format("2006-01-02")
				inMonth := current.Month() == time.Month(m)
				day := YearDay{
					Day:      current.Day(),
					InMonth:  inMonth,
					IsPeriod: periodDays[key],
					IsToday:  current.Equal(today),
				}
				if lastPeriodStart != nil && inMonth {
					info := calculateCycleInfo(*lastPeriodStart, cycleLength, periodLength, current)
					day.Phase = info.Phase
					if info.Phase == "menstruation" && !day.IsPeriod && current.After(today) {
						day.IsPredicted = true
					}
				}
				week = append(week, day)
				current = current.AddDate(0, 0, 1)
			}
			weeks = append(weeks, week)
		}

		months = append(months, YearMonth{
			Name:  firstOfMonth.Format("January"),
			Year:  year,
			Weeks: weeks,
		})
	}
	return months
}

// ─── Journal Word Cloud ─────────────────────────────────────────────────────

type WordFreq struct {
	Word  string
	Count int
	Size  int // CSS font size class 1-5
}

func getJournalWordCloud(userID int64, maxWords int) []WordFreq {
	stopWords := map[string]bool{
		"the": true, "a": true, "an": true, "and": true, "or": true, "but": true,
		"in": true, "on": true, "at": true, "to": true, "for": true, "of": true,
		"is": true, "it": true, "was": true, "be": true, "been": true, "being": true,
		"have": true, "has": true, "had": true, "do": true, "does": true, "did": true,
		"will": true, "would": true, "could": true, "should": true, "may": true, "can": true,
		"this": true, "that": true, "these": true, "those": true, "with": true, "from": true,
		"not": true, "no": true, "so": true, "if": true, "my": true, "me": true,
		"i": true, "we": true, "you": true, "he": true, "she": true, "they": true,
		"them": true, "its": true, "our": true, "your": true, "her": true, "his": true,
		"just": true, "very": true, "really": true, "also": true, "about": true,
		"than": true, "some": true, "more": true, "much": true, "too": true,
		"then": true, "now": true, "well": true, "like": true, "get": true, "got": true,
		"am": true, "are": true, "were": true, "what": true, "when": true, "where": true,
		"which": true, "who": true, "how": true, "all": true, "each": true, "both": true,
		"few": true, "many": true, "most": true, "other": true, "up": true, "out": true,
		"into": true, "over": true, "after": true, "before": true, "between": true,
		"under": true, "again": true, "once": true, "only": true, "same": true,
		"here": true, "there": true, "why": true, "because": true, "during": true,
		"until": true, "while": true, "day": true, "today": true, "feel": true,
		"feeling": true, "felt": true, "bit": true, "lot": true,
	}

	entries, err := getJournalEntries(userID, 10000)
	if err != nil || len(entries) == 0 {
		return nil
	}

	counts := make(map[string]int)
	for _, e := range entries {
		text := strings.ToLower(e.Content + " " + e.Title)
		// Remove non-alpha characters except spaces
		var cleaned strings.Builder
		for _, ch := range text {
			if (ch >= 'a' && ch <= 'z') || ch == ' ' {
				cleaned.WriteRune(ch)
			} else {
				cleaned.WriteRune(' ')
			}
		}
		words := strings.Fields(cleaned.String())
		for _, w := range words {
			if len(w) < 3 || stopWords[w] {
				continue
			}
			counts[w]++
		}
	}

	if len(counts) == 0 {
		return nil
	}

	// Sort by count descending
	type kv struct {
		k string
		v int
	}
	var sorted []kv
	for k, v := range counts {
		sorted = append(sorted, kv{k, v})
	}
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].v > sorted[j].v })

	if len(sorted) > maxWords {
		sorted = sorted[:maxWords]
	}

	maxCount := sorted[0].v
	var result []WordFreq
	for _, s := range sorted {
		size := 1
		if maxCount > 1 {
			ratio := float64(s.v) / float64(maxCount)
			switch {
			case ratio > 0.8:
				size = 5
			case ratio > 0.6:
				size = 4
			case ratio > 0.35:
				size = 3
			case ratio > 0.15:
				size = 2
			}
		}
		result = append(result, WordFreq{Word: s.k, Count: s.v, Size: size})
	}

	// Shuffle for visual variety (deterministic per day)
	day := time.Now().YearDay()
	for i := len(result) - 1; i > 0; i-- {
		j := (i * day) % (i + 1)
		result[i], result[j] = result[j], result[i]
	}

	return result
}

func scanDailyReadings(rows *sql.Rows) ([]DailyReading, error) {
	var readings []DailyReading
	for rows.Next() {
		var r DailyReading
		var dateStr string
		var basalTemp sql.NullFloat64
		var cervicalMucus, notes, createdAt sql.NullString
		var sleepQuality, stressLevel, energyLevel sql.NullInt64
		if err := rows.Scan(&r.ID, &r.UserID, &dateStr, &basalTemp, &cervicalMucus, &notes, &createdAt, &sleepQuality, &stressLevel, &energyLevel); err != nil {
			return nil, err
		}
		r.Date = parseDate(dateStr)
		if basalTemp.Valid {
			r.BasalTemp = basalTemp.Float64
		}
		if cervicalMucus.Valid {
			r.CervicalMucus = cervicalMucus.String
		}
		if notes.Valid {
			r.Notes = notes.String
		}
		if createdAt.Valid {
			r.CreatedAt = createdAt.String
		}
		if sleepQuality.Valid {
			r.SleepQuality = int(sleepQuality.Int64)
		}
		if stressLevel.Valid {
			r.StressLevel = int(stressLevel.Int64)
		}
		if energyLevel.Valid {
			r.EnergyLevel = int(energyLevel.Int64)
		}
		readings = append(readings, r)
	}
	return readings, rows.Err()
}
