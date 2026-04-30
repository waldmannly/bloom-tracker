package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/pbkdf2"
)

func handleHome(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	// Redirect logged-in users to dashboard
	if user := getSessionUser(r); user != nil {
		http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
		return
	}
	pd := &PageData{}
	renderTemplate(w, r, "home", pd)
}

// ─── Dashboard ──────────────────────────────────────────────────────────────

type dashboardData struct {
	CycleInfo      *CycleInfo
	ActivePeriod   *Period
	Predictions    []Prediction
	RecentSymptoms []Symptom
	HasPeriodData  bool
	IsPartner      bool
	PartnerName    string
	ShowFertility  bool
	SmartAlerts    []SmartAlert
	Pronouns       string
}

func handleDashboard(w http.ResponseWriter, r *http.Request) {
	user := getUserFromContext(r)
	pd := newPageData(r)

	// Determine whose cycle data to show
	targetUser := user
	isPartner := user.Role == "partner"
	partnerName := ""

	if isPartner && user.PartnerOf != nil {
		owner, err := getUserByID(*user.PartnerOf)
		if err != nil {
			setFlash(w, "error", "Could not load partner data")
			http.Redirect(w, r, "/settings", http.StatusSeeOther)
			return
		}
		targetUser = owner
		partnerName = owner.Name
	}

	lastPeriod, _ := getLastPeriod(targetUser.ID)
	activePeriod, _ := getActivePeriod(targetUser.ID)

	var cycleInfo *CycleInfo
	var predictions []Prediction

	if lastPeriod != nil {
		cycleInfo = calculateCycleInfo(lastPeriod.StartDate, targetUser.CycleLength, targetUser.PeriodLength, time.Now())
		predictions = predictFuturePeriods(lastPeriod.StartDate, targetUser.CycleLength, targetUser.PeriodLength, 3)
	}

	recentSymptoms, _ := getRecentSymptoms(targetUser.ID, 5)
	smartAlerts := generateSmartAlerts(targetUser.ID, targetUser.CycleLength, targetUser.PeriodLength)

	pd.Data = dashboardData{
		CycleInfo:      cycleInfo,
		ActivePeriod:   activePeriod,
		Predictions:    predictions,
		RecentSymptoms: recentSymptoms,
		HasPeriodData:  lastPeriod != nil,
		IsPartner:      isPartner,
		PartnerName:    partnerName,
		ShowFertility:  targetUser.ShowFertility,
		SmartAlerts:    smartAlerts,
		Pronouns:       targetUser.Pronouns,
	}

	if isPartner {
		renderTemplate(w, r, "partner-dashboard", pd)
	} else {
		renderTemplate(w, r, "dashboard", pd)
	}
}

// ─── Log Period ─────────────────────────────────────────────────────────────

type logPeriodData struct {
	Today        time.Time
	ActivePeriod *Period
	History      []Period
}

func handleLogPeriod(w http.ResponseWriter, r *http.Request) {
	user := getUserFromContext(r)

	if r.Method == http.MethodGet {
		pd := newPageData(r)
		activePeriod, _ := getActivePeriod(user.ID)
		history, _ := getPeriods(user.ID, 12)
		pd.Data = logPeriodData{
			Today:        time.Now(),
			ActivePeriod: activePeriod,
			History:      history,
		}
		renderTemplate(w, r, "log-period", pd)
		return
	}

	dateStr := r.FormValue("start_date")
	startDate, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		setFlash(w, "error", "Invalid date format")
		http.Redirect(w, r, "/log-period", http.StatusSeeOther)
		return
	}

	if startDate.After(time.Now()) {
		setFlash(w, "error", "Start date cannot be in the future")
		http.Redirect(w, r, "/log-period", http.StatusSeeOther)
		return
	}

	if err := logPeriodStart(user.ID, startDate); err != nil {
		setFlash(w, "error", "Could not log period. Please try again.")
		http.Redirect(w, r, "/log-period", http.StatusSeeOther)
		return
	}

	setFlash(w, "success", "Period logged! 🌺")
	notifyPartnerIfPhaseChanged(user.ID)
	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

func handleEndPeriod(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
		return
	}
	user := getUserFromContext(r)

	periodIDStr := r.FormValue("period_id")
	periodID, err := strconv.ParseInt(periodIDStr, 10, 64)
	if err != nil {
		setFlash(w, "error", "Invalid period")
		http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
		return
	}

	// Verify this period belongs to the user
	activePeriod, err := getActivePeriod(user.ID)
	if err != nil || activePeriod.ID != periodID {
		setFlash(w, "error", "Period not found")
		http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
		return
	}

	if err := endPeriod(periodID, time.Now()); err != nil {
		setFlash(w, "error", "Could not end period")
		http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
		return
	}

	setFlash(w, "success", "Period ended!")
	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

func handleEditPeriod(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/log-period", http.StatusSeeOther)
		return
	}
	user := getUserFromContext(r)

	periodIDStr := r.FormValue("period_id")
	periodID, err := strconv.ParseInt(periodIDStr, 10, 64)
	if err != nil {
		setFlash(w, "error", "Invalid period")
		http.Redirect(w, r, "/log-period", http.StatusSeeOther)
		return
	}

	// Verify ownership
	if _, err := getPeriodByID(periodID, user.ID); err != nil {
		setFlash(w, "error", "Period not found")
		http.Redirect(w, r, "/log-period", http.StatusSeeOther)
		return
	}

	startStr := r.FormValue("start_date")
	startDate, err := time.Parse("2006-01-02", startStr)
	if err != nil {
		setFlash(w, "error", "Invalid start date")
		http.Redirect(w, r, "/log-period", http.StatusSeeOther)
		return
	}

	var endDate *time.Time
	endStr := r.FormValue("end_date")
	if endStr != "" {
		t, err := time.Parse("2006-01-02", endStr)
		if err == nil {
			if t.Before(startDate) {
				setFlash(w, "error", "End date cannot be before start date")
				http.Redirect(w, r, "/log-period", http.StatusSeeOther)
				return
			}
			endDate = &t
		}
	}

	if err := updatePeriod(periodID, startDate, endDate); err != nil {
		setFlash(w, "error", "Could not update period")
		http.Redirect(w, r, "/log-period", http.StatusSeeOther)
		return
	}

	setFlash(w, "success", "Period updated! ✏️")
	http.Redirect(w, r, "/log-period", http.StatusSeeOther)
}

func handleDeletePeriod(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/log-period", http.StatusSeeOther)
		return
	}
	user := getUserFromContext(r)

	periodIDStr := r.FormValue("period_id")
	periodID, err := strconv.ParseInt(periodIDStr, 10, 64)
	if err != nil {
		setFlash(w, "error", "Invalid period")
		http.Redirect(w, r, "/log-period", http.StatusSeeOther)
		return
	}

	if err := deletePeriod(periodID, user.ID); err != nil {
		setFlash(w, "error", "Could not delete period")
		http.Redirect(w, r, "/log-period", http.StatusSeeOther)
		return
	}

	setFlash(w, "success", "Period deleted")
	http.Redirect(w, r, "/log-period", http.StatusSeeOther)
}

// ─── Symptoms ───────────────────────────────────────────────────────────────

type symptomsData struct {
	Today         time.Time
	TodaySymptoms []Symptom
}

func handleSymptoms(w http.ResponseWriter, r *http.Request) {
	user := getUserFromContext(r)

	if r.Method == http.MethodGet {
		pd := newPageData(r)
		today := midnight(time.Now())
		todaySymptoms, _ := getSymptomsForDate(user.ID, today)
		pd.Data = symptomsData{
			Today:         today,
			TodaySymptoms: todaySymptoms,
		}
		renderTemplate(w, r, "symptoms", pd)
		return
	}

	dateStr := r.FormValue("date")
	date, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		date = time.Now()
	}

	category := strings.TrimSpace(r.FormValue("category"))
	symptomsStr := strings.TrimSpace(r.FormValue("symptoms"))
	customSymptom := strings.TrimSpace(r.FormValue("custom_symptom"))
	severityStr := r.FormValue("severity")
	notes := strings.TrimSpace(r.FormValue("notes"))

	// Build list of symptoms to log
	var symptomNames []string
	if symptomsStr != "" {
		for _, s := range strings.Split(symptomsStr, ",") {
			s = strings.TrimSpace(s)
			if s != "" {
				symptomNames = append(symptomNames, s)
			}
		}
	}
	// Also accept legacy single symptom field
	if single := strings.TrimSpace(r.FormValue("symptom")); single != "" && len(symptomNames) == 0 {
		symptomNames = append(symptomNames, single)
	}
	if customSymptom != "" {
		symptomNames = append(symptomNames, customSymptom)
	}

	if category == "" || len(symptomNames) == 0 {
		setFlash(w, "error", "Please select a category and at least one symptom")
		http.Redirect(w, r, "/symptoms", http.StatusSeeOther)
		return
	}

	severity, err := strconv.Atoi(severityStr)
	if err != nil || severity < 1 || severity > 5 {
		severity = 3
	}

	var maxSeveritySymptom string
	for _, symptom := range symptomNames {
		if err := logSymptom(user.ID, date, category, symptom, severity, notes); err != nil {
			continue
		}
		maxSeveritySymptom = symptom
	}

	// Notify partner for severe symptoms (use last one logged)
	if maxSeveritySymptom != "" {
		go notifyPartnerOfSymptoms(user.ID, maxSeveritySymptom, category, severity)
	}

	count := len(symptomNames)
	if count == 1 {
		setFlash(w, "success", "Symptom logged! 📝")
	} else {
		setFlash(w, "success", fmt.Sprintf("%d symptoms logged! 📝", count))
	}
	http.Redirect(w, r, "/symptoms", http.StatusSeeOther)
}

func handleDeleteSymptom(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/symptoms", http.StatusSeeOther)
		return
	}
	user := getUserFromContext(r)

	idStr := r.FormValue("symptom_id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		setFlash(w, "error", "Invalid symptom")
		http.Redirect(w, r, "/symptoms", http.StatusSeeOther)
		return
	}

	if err := deleteSymptom(id, user.ID); err != nil {
		setFlash(w, "error", "Could not delete symptom")
	}

	http.Redirect(w, r, "/symptoms", http.StatusSeeOther)
}

// ─── Calendar ───────────────────────────────────────────────────────────────

func handleCalendar(w http.ResponseWriter, r *http.Request) {
	user := getUserFromContext(r)
	pd := newPageData(r)

	now := time.Now()
	year := now.Year()
	month := int(now.Month())

	if y := r.URL.Query().Get("year"); y != "" {
		if v, err := strconv.Atoi(y); err == nil {
			year = v
		}
	}
	if m := r.URL.Query().Get("month"); m != "" {
		if v, err := strconv.Atoi(m); err == nil && v >= 1 && v <= 12 {
			month = v
		}
	}

	var lastPeriodStart *time.Time
	if lp, err := getLastPeriod(user.ID); err == nil {
		lastPeriodStart = &lp.StartDate
	}

	cal := generateCalendar(year, month, user.ID, user.CycleLength, user.PeriodLength, lastPeriodStart)
	pd.Data = cal

	renderTemplate(w, r, "calendar", pd)
}

// ─── Settings ───────────────────────────────────────────────────────────────

type settingsData struct {
	CycleLength       int
	PeriodLength      int
	PartnerCode       string
	PartnerName       string
	HasPartner        bool
	PartnerNotify     bool
	ShowFertility     bool
	Pronouns          string
	Theme             string
	PhasePrefs        map[string]string
	EncryptionEnabled bool
}

func handleSettings(w http.ResponseWriter, r *http.Request) {
	user := getUserFromContext(r)

	if r.Method == http.MethodGet {
		pd := newPageData(r)

		data := settingsData{
			CycleLength:       user.CycleLength,
			PeriodLength:      user.PeriodLength,
			PartnerCode:       user.PartnerCode,
			PartnerNotify:     user.PartnerNotify,
			ShowFertility:     user.ShowFertility,
			Pronouns:          user.Pronouns,
			Theme:             user.Theme,
			PhasePrefs:        getPhasePreferences(user.ID),
			EncryptionEnabled: dbEncryptionEnabled,
		}

		if user.Role == "owner" {
			if partner, err := getPartnerForOwner(user.ID); err == nil {
				data.HasPartner = true
				data.PartnerName = partner.Name
			}
		}

		pd.Data = data
		renderTemplate(w, r, "settings", pd)
		return
	}

	cycleLenStr := r.FormValue("cycle_length")
	periodLenStr := r.FormValue("period_length")

	cycleLen, err := strconv.Atoi(cycleLenStr)
	if err != nil || cycleLen < 20 || cycleLen > 45 {
		cycleLen = user.CycleLength
	}

	periodLen, err := strconv.Atoi(periodLenStr)
	if err != nil || periodLen < 1 || periodLen > 10 {
		periodLen = user.PeriodLength
	}

	showFertility := r.FormValue("show_fertility") == "on"
	pronouns := r.FormValue("pronouns")
	if pronouns == "" {
		pronouns = user.Pronouns
	}

	theme := r.FormValue("theme")
	if theme != "bloom" && theme != "ocean" {
		theme = "bloom"
	}

	if err := updateUserSettings(user.ID, cycleLen, periodLen, showFertility, pronouns, theme); err != nil {
		setFlash(w, "error", "Could not save settings")
		http.Redirect(w, r, "/settings", http.StatusSeeOther)
		return
	}

	setFlash(w, "success", "Settings saved!")
	http.Redirect(w, r, "/settings", http.StatusSeeOther)
}

func handlePhasePreferences(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/settings", http.StatusSeeOther)
		return
	}
	user := getUserFromContext(r)

	phases := []string{"menstruation", "follicular", "ovulation", "luteal"}
	for _, phase := range phases {
		pref := strings.TrimSpace(r.FormValue("pref_" + phase))
		savePhasePreference(user.ID, phase, pref)
	}

	setFlash(w, "success", "Phase preferences saved! 💛")
	http.Redirect(w, r, "/settings", http.StatusSeeOther)
}

// ─── Partner ────────────────────────────────────────────────────────────────

func handlePartnerInvite(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/settings", http.StatusSeeOther)
		return
	}
	user := getUserFromContext(r)

	code := generateInviteCode()
	if err := setPartnerCode(user.ID, code); err != nil {
		setFlash(w, "error", "Could not generate invite code")
		http.Redirect(w, r, "/settings", http.StatusSeeOther)
		return
	}

	setFlash(w, "success", fmt.Sprintf("Invite code generated: %s", code))
	http.Redirect(w, r, "/settings", http.StatusSeeOther)
}

func handlePartnerJoin(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		pd := &PageData{}
		renderTemplate(w, r, "partner-join", pd)
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	email := strings.TrimSpace(strings.ToLower(r.FormValue("email")))
	password := r.FormValue("password")
	code := strings.TrimSpace(strings.ToUpper(r.FormValue("code")))

	if name == "" || email == "" || password == "" || code == "" {
		pd := &PageData{Flash: "Please fill in all fields", FlashType: "error"}
		renderTemplate(w, r, "partner-join", pd)
		return
	}

	if len(password) < 8 {
		pd := &PageData{Flash: "Password must be at least 8 characters", FlashType: "error"}
		renderTemplate(w, r, "partner-join", pd)
		return
	}

	// Find the owner with this invite code
	owner, err := getUserByPartnerCode(code)
	if err != nil {
		pd := &PageData{Flash: "Invalid invite code", FlashType: "error"}
		renderTemplate(w, r, "partner-join", pd)
		return
	}

	hash, err := hashPassword(password)
	if err != nil {
		pd := &PageData{Flash: "Something went wrong", FlashType: "error"}
		renderTemplate(w, r, "partner-join", pd)
		return
	}

	partnerID, err := createUser(email, name, hash, "partner")
	if err != nil {
		pd := &PageData{}
		if strings.Contains(err.Error(), "UNIQUE") {
			pd.Flash = "An account with this email already exists"
		} else {
			pd.Flash = "Could not create account"
		}
		pd.FlashType = "error"
		renderTemplate(w, r, "partner-join", pd)
		return
	}

	if err := linkPartner(partnerID, owner.ID); err != nil {
		pd := &PageData{Flash: "Could not link to partner", FlashType: "error"}
		renderTemplate(w, r, "partner-join", pd)
		return
	}

	// Clear the invite code so it can't be reused
	setPartnerCode(owner.ID, "")

	// Enable partner notifications by default
	setPartnerNotify(owner.ID, true)

	setFlash(w, "success", fmt.Sprintf("You're now connected to %s! Please log in.", owner.Name))
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

// ─── Notifications ──────────────────────────────────────────────────────

func handleNotifications(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/settings", http.StatusSeeOther)
		return
	}
	user := getUserFromContext(r)

	enabled := r.FormValue("partner_notify") == "on"
	if err := setPartnerNotify(user.ID, enabled); err != nil {
		setFlash(w, "error", "Could not update notification settings")
	} else {
		if enabled {
			setFlash(w, "success", "Partner notifications enabled! 📧")
		} else {
			setFlash(w, "success", "Partner notifications disabled")
		}
	}
	http.Redirect(w, r, "/settings", http.StatusSeeOther)
}

// ─── Trends ─────────────────────────────────────────────────────────────────

type trendsData struct {
	CycleTrends       []CycleTrend
	TopSymptoms       []SymptomTrend
	MonthSummaries    []MonthSummary
	PhaseCorrelations []PhaseSymptomCorrelation
	BBTReadings       []DailyReading
	TotalPeriods      int
	TotalSymptoms     int
	AvgCycleLen       float64
	HasData           bool
	MaxCycleLen       int
	YearMonths        []YearMonth
	WordCloud         []WordFreq
	WellnessData      []DailyReading
}

func handleTrends(w http.ResponseWriter, r *http.Request) {
	user := getUserFromContext(r)
	pd := newPageData(r)

	cycleTrends, _ := getCycleTrends(user.ID, 24)
	topSymptoms, _ := getTopSymptoms(user.ID, 10)
	monthSummaries, _ := getMonthSummaries(user.ID, 12)
	phaseCorrelations, _ := getSymptomPhaseCorrelations(user.ID, user.CycleLength, user.PeriodLength)

	// BBT readings for trends (all available)
	bbtReadings, _ := getDailyReadings(user.ID, 90)

	// Wellness data: last 30 days, reversed to chronological order
	wellnessReadings, _ := getDailyReadings(user.ID, 30)
	for i, j := 0, len(wellnessReadings)-1; i < j; i, j = i+1, j-1 {
		wellnessReadings[i], wellnessReadings[j] = wellnessReadings[j], wellnessReadings[i]
	}

	// Year-at-a-Glance calendar
	var lastPeriodStart *time.Time
	if lp, err := getLastPeriod(user.ID); err == nil {
		lastPeriodStart = &lp.StartDate
	}
	yearMonths := generateYearCalendar(user.ID, user.CycleLength, user.PeriodLength, lastPeriodStart)

	// Journal word cloud
	wordCloud := getJournalWordCloud(user.ID, 40)

	maxCycle := 0
	for _, ct := range cycleTrends {
		if ct.CycleLength > maxCycle {
			maxCycle = ct.CycleLength
		}
	}
	if maxCycle == 0 {
		maxCycle = 35
	}

	pd.Data = trendsData{
		CycleTrends:       cycleTrends,
		TopSymptoms:       topSymptoms,
		MonthSummaries:    monthSummaries,
		PhaseCorrelations: phaseCorrelations,
		BBTReadings:       bbtReadings,
		TotalPeriods:      getTotalPeriodCount(user.ID),
		TotalSymptoms:     getTotalSymptomCount(user.ID),
		AvgCycleLen:       getAverageCycleLength(user.ID),
		HasData:           len(cycleTrends) > 0 || len(topSymptoms) > 0,
		MaxCycleLen:       maxCycle,
		YearMonths:        yearMonths,
		WordCloud:         wordCloud,
		WellnessData:      wellnessReadings,
	}

	renderTemplate(w, r, "trends", pd)
}

// ─── Bulk Import ────────────────────────────────────────────────────────────

type importData struct {
	ImportCount int
	Errors      []string
	History     []Period
}

func handleImport(w http.ResponseWriter, r *http.Request) {
	user := getUserFromContext(r)

	if r.Method == http.MethodGet {
		pd := newPageData(r)
		history, _ := getPeriods(user.ID, 50)
		pd.Data = importData{History: history}
		renderTemplate(w, r, "import", pd)
		return
	}

	rawInput := r.FormValue("periods")
	lines := strings.Split(rawInput, "\n")

	imported := 0
	var errors []string

	for i, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Support formats:
		// 2024-01-15
		// 2024-01-15 to 2024-01-20
		// 2024-01-15 - 2024-01-20
		// 2024-01-15, 2024-01-20
		var startStr, endStr string

		for _, sep := range []string{" to ", " - ", ", "} {
			if parts := strings.SplitN(line, sep, 2); len(parts) == 2 {
				startStr = strings.TrimSpace(parts[0])
				endStr = strings.TrimSpace(parts[1])
				break
			}
		}
		if startStr == "" {
			startStr = line
		}

		startDate, err := parseFlexDate(startStr)
		if err != nil {
			errors = append(errors, fmt.Sprintf("Line %d: invalid start date '%s'", i+1, startStr))
			continue
		}

		if startDate.After(time.Now()) {
			errors = append(errors, fmt.Sprintf("Line %d: date is in the future", i+1))
			continue
		}

		if err := logPeriodStart(user.ID, startDate); err != nil {
			errors = append(errors, fmt.Sprintf("Line %d: could not save — %v", i+1, err))
			continue
		}

		if endStr != "" {
			endDate, err := parseFlexDate(endStr)
			if err != nil {
				errors = append(errors, fmt.Sprintf("Line %d: invalid end date '%s'", i+1, endStr))
			} else {
				// Get the period we just created and end it
				if active, err := getActivePeriod(user.ID); err == nil {
					endPeriod(active.ID, endDate)
				}
			}
		}

		imported++
	}

	if imported > 0 {
		setFlash(w, "success", fmt.Sprintf("Imported %d periods! 🌺", imported))
	}
	if len(errors) > 0 && imported == 0 {
		setFlash(w, "error", fmt.Sprintf("%d errors — check the format below", len(errors)))
	}

	pd := newPageData(r)
	history, _ := getPeriods(user.ID, 50)
	pd.Data = importData{
		ImportCount: imported,
		Errors:      errors,
		History:     history,
	}
	renderTemplate(w, r, "import", pd)
}

func parseFlexDate(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	formats := []string{
		"2006-01-02",
		"01/02/2006",
		"1/2/2006",
		"Jan 2, 2006",
		"January 2, 2006",
		"2 Jan 2006",
		"02-01-2006",
	}
	for _, f := range formats {
		if t, err := time.Parse(f, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unrecognized date format: %s", s)
}

// ─── Journal ────────────────────────────────────────────────────────────────

type journalData struct {
	Entries []JournalEntry
	Today   time.Time
}

func handleJournal(w http.ResponseWriter, r *http.Request) {
	user := getUserFromContext(r)

	if r.Method == http.MethodGet {
		pd := newPageData(r)
		entries, _ := getJournalEntries(user.ID, 50)
		pd.Data = journalData{
			Entries: entries,
			Today:   time.Now(),
		}
		renderTemplate(w, r, "journal", pd)
		return
	}

	dateStr := r.FormValue("date")
	date, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		date = time.Now()
	}

	moodEmoji := strings.TrimSpace(r.FormValue("mood"))
	title := strings.TrimSpace(r.FormValue("title"))
	content := strings.TrimSpace(r.FormValue("content"))

	if content == "" && title == "" {
		setFlash(w, "error", "Please write something in your journal entry")
		http.Redirect(w, r, "/journal", http.StatusSeeOther)
		return
	}

	if err := saveJournalEntry(user.ID, date, moodEmoji, title, content); err != nil {
		setFlash(w, "error", "Could not save journal entry")
		http.Redirect(w, r, "/journal", http.StatusSeeOther)
		return
	}

	setFlash(w, "success", "Journal entry saved! 📔")
	http.Redirect(w, r, "/journal", http.StatusSeeOther)
}

func handleDeleteJournal(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/journal", http.StatusSeeOther)
		return
	}
	user := getUserFromContext(r)
	idStr := r.FormValue("entry_id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		setFlash(w, "error", "Invalid entry")
		http.Redirect(w, r, "/journal", http.StatusSeeOther)
		return
	}
	deleteJournalEntry(id, user.ID)
	http.Redirect(w, r, "/journal", http.StatusSeeOther)
}

// ─── Privacy Page ───────────────────────────────────────────────────────────

func handlePrivacy(w http.ResponseWriter, r *http.Request) {
	pd := newPageData(r)
	renderTemplate(w, r, "privacy", pd)
}

// ─── Data Export ────────────────────────────────────────────────────────────

func handleExport(w http.ResponseWriter, r *http.Request) {
	user := getUserFromContext(r)
	format := r.URL.Query().Get("format")

	periods, _ := getAllPeriods(user.ID)
	symptoms, _ := getAllSymptoms(user.ID)
	journal, _ := getAllJournalEntries(user.ID)
	readings, _ := getAllDailyReadings(user.ID)

	switch format {
	case "json":
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Disposition", "attachment; filename=bloom-export.json")
		type exportJSON struct {
			User struct {
				Name         string `json:"name"`
				Email        string `json:"email"`
				CycleLength  int    `json:"cycle_length"`
				PeriodLength int    `json:"period_length"`
				Pronouns     string `json:"pronouns"`
			} `json:"user"`
			Periods []struct {
				StartDate string `json:"start_date"`
				EndDate   string `json:"end_date,omitempty"`
			} `json:"periods"`
			Symptoms []struct {
				Date     string `json:"date"`
				Category string `json:"category"`
				Symptom  string `json:"symptom"`
				Severity int    `json:"severity"`
				Notes    string `json:"notes,omitempty"`
			} `json:"symptoms"`
			Journal []struct {
				Date    string `json:"date"`
				Mood    string `json:"mood,omitempty"`
				Title   string `json:"title,omitempty"`
				Content string `json:"content"`
			} `json:"journal"`
			DailyReadings []struct {
				Date          string  `json:"date"`
				BasalTemp     float64 `json:"basal_temp,omitempty"`
				CervicalMucus string  `json:"cervical_mucus,omitempty"`
				Notes         string  `json:"notes,omitempty"`
				SleepQuality  int     `json:"sleep_quality,omitempty"`
				StressLevel   int     `json:"stress_level,omitempty"`
				EnergyLevel   int     `json:"energy_level,omitempty"`
			} `json:"daily_readings"`
		}
		var data exportJSON
		data.User.Name = user.Name
		data.User.Email = user.Email
		data.User.CycleLength = user.CycleLength
		data.User.PeriodLength = user.PeriodLength
		data.User.Pronouns = user.Pronouns
		for _, p := range periods {
			entry := struct {
				StartDate string `json:"start_date"`
				EndDate   string `json:"end_date,omitempty"`
			}{StartDate: p.StartDate.Format("2006-01-02")}
			if p.EndDate != nil {
				entry.EndDate = p.EndDate.Format("2006-01-02")
			}
			data.Periods = append(data.Periods, entry)
		}
		for _, s := range symptoms {
			data.Symptoms = append(data.Symptoms, struct {
				Date     string `json:"date"`
				Category string `json:"category"`
				Symptom  string `json:"symptom"`
				Severity int    `json:"severity"`
				Notes    string `json:"notes,omitempty"`
			}{s.Date.Format("2006-01-02"), s.Category, s.Name, s.Severity, s.Notes})
		}
		for _, j := range journal {
			data.Journal = append(data.Journal, struct {
				Date    string `json:"date"`
				Mood    string `json:"mood,omitempty"`
				Title   string `json:"title,omitempty"`
				Content string `json:"content"`
			}{j.Date.Format("2006-01-02"), j.MoodEmoji, j.Title, j.Content})
		}
		for _, dr := range readings {
			data.DailyReadings = append(data.DailyReadings, struct {
				Date          string  `json:"date"`
				BasalTemp     float64 `json:"basal_temp,omitempty"`
				CervicalMucus string  `json:"cervical_mucus,omitempty"`
				Notes         string  `json:"notes,omitempty"`
				SleepQuality  int     `json:"sleep_quality,omitempty"`
				StressLevel   int     `json:"stress_level,omitempty"`
				EnergyLevel   int     `json:"energy_level,omitempty"`
			}{dr.Date.Format("2006-01-02"), dr.BasalTemp, dr.CervicalMucus, dr.Notes, dr.SleepQuality, dr.StressLevel, dr.EnergyLevel})
		}
		json.NewEncoder(w).Encode(data)

	default: // CSV
		w.Header().Set("Content-Type", "text/csv")
		w.Header().Set("Content-Disposition", "attachment; filename=bloom-export.csv")
		cw := csv.NewWriter(w)
		cw.Write([]string{"--- PERIODS ---"})
		cw.Write([]string{"Start Date", "End Date"})
		for _, p := range periods {
			end := ""
			if p.EndDate != nil {
				end = p.EndDate.Format("2006-01-02")
			}
			cw.Write([]string{p.StartDate.Format("2006-01-02"), end})
		}
		cw.Write([]string{""})
		cw.Write([]string{"--- SYMPTOMS ---"})
		cw.Write([]string{"Date", "Category", "Symptom", "Severity", "Notes"})
		for _, s := range symptoms {
			cw.Write([]string{s.Date.Format("2006-01-02"), s.Category, s.Name, strconv.Itoa(s.Severity), s.Notes})
		}
		cw.Write([]string{""})
		cw.Write([]string{"--- JOURNAL ---"})
		cw.Write([]string{"Date", "Mood", "Title", "Content"})
		for _, j := range journal {
			cw.Write([]string{j.Date.Format("2006-01-02"), j.MoodEmoji, j.Title, j.Content})
		}
		cw.Write([]string{""})
		cw.Write([]string{"--- DAILY READINGS ---"})
		cw.Write([]string{"Date", "Basal Temp (°C)", "Cervical Mucus", "Notes", "Sleep Quality", "Stress Level", "Energy Level"})
		for _, dr := range readings {
			temp := ""
			if dr.BasalTemp > 0 {
				temp = fmt.Sprintf("%.2f", dr.BasalTemp)
			}
			sleep := ""
			if dr.SleepQuality > 0 {
				sleep = strconv.Itoa(dr.SleepQuality)
			}
			stress := ""
			if dr.StressLevel > 0 {
				stress = strconv.Itoa(dr.StressLevel)
			}
			energy := ""
			if dr.EnergyLevel > 0 {
				energy = strconv.Itoa(dr.EnergyLevel)
			}
			cw.Write([]string{dr.Date.Format("2006-01-02"), temp, dr.CervicalMucus, dr.Notes, sleep, stress, energy})
		}
		cw.Flush()
	}
}

// ─── Account Deletion ──────────────────────────────────────────────────────

func handleDeleteAccount(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/settings", http.StatusSeeOther)
		return
	}
	user := getUserFromContext(r)

	password := r.FormValue("password")
	if !checkPassword(user.PasswordHash, password) {
		setFlash(w, "error", "Incorrect password. Account not deleted.")
		http.Redirect(w, r, "/settings", http.StatusSeeOther)
		return
	}

	if err := deleteUserAccount(user.ID); err != nil {
		setFlash(w, "error", "Could not delete account. Please try again.")
		http.Redirect(w, r, "/settings", http.StatusSeeOther)
		return
	}

	// Clear session
	if cookie, err := r.Cookie("session"); err == nil {
		deleteSession(cookie.Value)
	}
	http.SetCookie(w, &http.Cookie{
		Name: "session", Value: "", Path: "/", MaxAge: -1, HttpOnly: true,
	})

	setFlash(w, "success", "Your account and all data have been permanently deleted. Take care. 🌸")
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// ─── Daily Log (BBT + Cervical Mucus) ──────────────────────────────────────

type dailyLogData struct {
	Today    time.Time
	Readings []DailyReading
}

func handleDailyLog(w http.ResponseWriter, r *http.Request) {
	user := getUserFromContext(r)

	if r.Method == http.MethodGet {
		pd := newPageData(r)
		readings, _ := getDailyReadings(user.ID, 30)
		pd.Data = dailyLogData{
			Today:    time.Now(),
			Readings: readings,
		}
		renderTemplate(w, r, "daily-log", pd)
		return
	}

	dateStr := r.FormValue("date")
	date, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		date = time.Now()
	}

	tempStr := r.FormValue("basal_temp")
	basalTemp := 0.0
	if tempStr != "" {
		basalTemp, _ = strconv.ParseFloat(tempStr, 64)
	}

	cervicalMucus := r.FormValue("cervical_mucus")
	notes := strings.TrimSpace(r.FormValue("notes"))

	sleepQuality, _ := strconv.Atoi(r.FormValue("sleep_quality"))
	stressLevel, _ := strconv.Atoi(r.FormValue("stress_level"))
	energyLevel, _ := strconv.Atoi(r.FormValue("energy_level"))
	if sleepQuality < 0 || sleepQuality > 5 {
		sleepQuality = 0
	}
	if stressLevel < 0 || stressLevel > 5 {
		stressLevel = 0
	}
	if energyLevel < 0 || energyLevel > 5 {
		energyLevel = 0
	}

	if basalTemp == 0 && cervicalMucus == "" && sleepQuality == 0 && stressLevel == 0 && energyLevel == 0 {
		setFlash(w, "error", "Please enter at least a temperature, cervical mucus observation, or wellness metric")
		http.Redirect(w, r, "/daily-log", http.StatusSeeOther)
		return
	}

	if basalTemp > 0 && (basalTemp < 35.0 || basalTemp > 40.0) {
		setFlash(w, "error", "Temperature should be between 35.0°C and 40.0°C")
		http.Redirect(w, r, "/daily-log", http.StatusSeeOther)
		return
	}

	if err := saveDailyReading(user.ID, date, basalTemp, cervicalMucus, notes, sleepQuality, stressLevel, energyLevel); err != nil {
		setFlash(w, "error", "Could not save daily reading")
		http.Redirect(w, r, "/daily-log", http.StatusSeeOther)
		return
	}

	setFlash(w, "success", "Daily reading saved! 🌡️")
	http.Redirect(w, r, "/daily-log", http.StatusSeeOther)
}

func handleDeleteDailyLog(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/daily-log", http.StatusSeeOther)
		return
	}
	user := getUserFromContext(r)
	idStr := r.FormValue("reading_id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		setFlash(w, "error", "Invalid reading")
		http.Redirect(w, r, "/daily-log", http.StatusSeeOther)
		return
	}
	deleteDailyReading(id, user.ID)
	http.Redirect(w, r, "/daily-log", http.StatusSeeOther)
}

// ─── Encrypted Backup ──────────────────────────────────────────────────────

func handleBackup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/settings", http.StatusSeeOther)
		return
	}
	user := getUserFromContext(r)

	password := r.FormValue("backup_password")
	if len(password) < 8 {
		setFlash(w, "error", "Backup password must be at least 8 characters")
		http.Redirect(w, r, "/settings", http.StatusSeeOther)
		return
	}

	// Collect all data as JSON
	periods, _ := getAllPeriods(user.ID)
	symptoms, _ := getAllSymptoms(user.ID)
	journal, _ := getAllJournalEntries(user.ID)
	readings, _ := getAllDailyReadings(user.ID)

	backupData := map[string]interface{}{
		"version":     1,
		"exported_at": time.Now().Format(time.RFC3339),
		"user": map[string]interface{}{
			"name":          user.Name,
			"email":         user.Email,
			"cycle_length":  user.CycleLength,
			"period_length": user.PeriodLength,
			"pronouns":      user.Pronouns,
		},
		"periods":        periods,
		"symptoms":       symptoms,
		"journal":        journal,
		"daily_readings": readings,
	}

	plaintext, err := json.Marshal(backupData)
	if err != nil {
		setFlash(w, "error", "Could not create backup")
		http.Redirect(w, r, "/settings", http.StatusSeeOther)
		return
	}

	// Derive key: PBKDF2(password, salt, 100000 iterations, SHA-256) → 32-byte AES key
	salt := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		setFlash(w, "error", "Encryption error")
		http.Redirect(w, r, "/settings", http.StatusSeeOther)
		return
	}
	key := pbkdf2.Key([]byte(password), salt, 100000, 32, sha256.New)

	block, err := aes.NewCipher(key)
	if err != nil {
		setFlash(w, "error", "Encryption error")
		http.Redirect(w, r, "/settings", http.StatusSeeOther)
		return
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		setFlash(w, "error", "Encryption error")
		http.Redirect(w, r, "/settings", http.StatusSeeOther)
		return
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		setFlash(w, "error", "Encryption error")
		http.Redirect(w, r, "/settings", http.StatusSeeOther)
		return
	}

	ciphertext := gcm.Seal(nil, nonce, plaintext, nil)

	// Output: "BLOOM" (5) + version (1) + salt (16) + nonce (12) + ciphertext
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=bloom-backup-%s.enc", time.Now().Format("2006-01-02")))
	w.Write([]byte("BLOOM"))
	w.Write([]byte{0x01}) // version
	w.Write(salt)
	w.Write(nonce)
	w.Write(ciphertext)
}

func handleRestore(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/settings", http.StatusSeeOther)
		return
	}
	user := getUserFromContext(r)

	password := r.FormValue("restore_password")
	file, _, err := r.FormFile("backup_file")
	if err != nil {
		setFlash(w, "error", "Please select a backup file")
		http.Redirect(w, r, "/settings", http.StatusSeeOther)
		return
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, 50*1024*1024)) // 50MB limit
	if err != nil {
		setFlash(w, "error", "Could not read backup file")
		http.Redirect(w, r, "/settings", http.StatusSeeOther)
		return
	}

	// Parse header: "BLOOM" (5) + version (1) + salt (16) + nonce (12) + ciphertext
	if len(data) < 34 || string(data[:5]) != "BLOOM" {
		setFlash(w, "error", "Invalid backup file format")
		http.Redirect(w, r, "/settings", http.StatusSeeOther)
		return
	}

	salt := data[6:22]
	key := pbkdf2.Key([]byte(password), salt, 100000, 32, sha256.New)

	block, err := aes.NewCipher(key)
	if err != nil {
		setFlash(w, "error", "Decryption error")
		http.Redirect(w, r, "/settings", http.StatusSeeOther)
		return
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		setFlash(w, "error", "Decryption error")
		http.Redirect(w, r, "/settings", http.StatusSeeOther)
		return
	}

	nonceSize := gcm.NonceSize()
	if len(data) < 22+nonceSize {
		setFlash(w, "error", "Invalid backup file")
		http.Redirect(w, r, "/settings", http.StatusSeeOther)
		return
	}

	nonce := data[22 : 22+nonceSize]
	ciphertext := data[22+nonceSize:]

	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		setFlash(w, "error", "Wrong password or corrupted backup file")
		http.Redirect(w, r, "/settings", http.StatusSeeOther)
		return
	}

	// Parse the decrypted JSON
	var backup struct {
		Periods []struct {
			StartDate string `json:"StartDate"`
			EndDate   string `json:"EndDate"`
		} `json:"periods"`
		Symptoms []struct {
			Date     string `json:"Date"`
			Category string `json:"Category"`
			Name     string `json:"Name"`
			Severity int    `json:"Severity"`
			Notes    string `json:"Notes"`
		} `json:"symptoms"`
		Journal []struct {
			Date      string `json:"Date"`
			MoodEmoji string `json:"MoodEmoji"`
			Title     string `json:"Title"`
			Content   string `json:"Content"`
		} `json:"journal"`
		DailyReadings []struct {
			Date          string  `json:"Date"`
			BasalTemp     float64 `json:"BasalTemp"`
			CervicalMucus string  `json:"CervicalMucus"`
			Notes         string  `json:"Notes"`
			SleepQuality  int     `json:"SleepQuality"`
			StressLevel   int     `json:"StressLevel"`
			EnergyLevel   int     `json:"EnergyLevel"`
		} `json:"daily_readings"`
	}

	if err := json.Unmarshal(plaintext, &backup); err != nil {
		setFlash(w, "error", "Could not parse backup data")
		http.Redirect(w, r, "/settings", http.StatusSeeOther)
		return
	}

	// Import the data
	imported := 0
	for _, p := range backup.Periods {
		startDate := parseDate(p.StartDate)
		if startDate.IsZero() {
			continue
		}
		if err := logPeriodStart(user.ID, startDate); err == nil {
			imported++
			if p.EndDate != "" {
				endDate := parseDate(p.EndDate)
				if !endDate.IsZero() {
					if active, err := getActivePeriod(user.ID); err == nil {
						endPeriod(active.ID, endDate)
					}
				}
			}
		}
	}
	for _, s := range backup.Symptoms {
		d := parseDate(s.Date)
		if !d.IsZero() {
			logSymptom(user.ID, d, s.Category, s.Name, s.Severity, s.Notes)
			imported++
		}
	}
	for _, j := range backup.Journal {
		d := parseDate(j.Date)
		if !d.IsZero() {
			saveJournalEntry(user.ID, d, j.MoodEmoji, j.Title, j.Content)
			imported++
		}
	}
	for _, dr := range backup.DailyReadings {
		d := parseDate(dr.Date)
		if !d.IsZero() {
			saveDailyReading(user.ID, d, dr.BasalTemp, dr.CervicalMucus, dr.Notes, dr.SleepQuality, dr.StressLevel, dr.EnergyLevel)
			imported++
		}
	}

	setFlash(w, "success", fmt.Sprintf("Restored %d records from backup! 🔐", imported))
	http.Redirect(w, r, "/settings", http.StatusSeeOther)
}

// ─── Import Template Download ──────────────────────────────────────────────

func handleImportTemplate(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", "attachment; filename=bloom-import-template.csv")
	cw := csv.NewWriter(w)
	cw.Write([]string{"# Bloom Period Import Template"})
	cw.Write([]string{"# One period per line. Use any of these formats:"})
	cw.Write([]string{"#"})
	cw.Write([]string{"# Format 1: YYYY-MM-DD to YYYY-MM-DD   (start to end)"})
	cw.Write([]string{"# Format 2: MM/DD/YYYY to MM/DD/YYYY"})
	cw.Write([]string{"# Format 3: Jan 2, 2024 to Jan 7, 2024"})
	cw.Write([]string{"# Format 4: YYYY-MM-DD                 (start only)"})
	cw.Write([]string{"# Separators: ' to ', ' - ', ', '"})
	cw.Write([]string{"#"})
	cw.Write([]string{"# Example data (remove # to use):"})
	cw.Write([]string{"# 2024-01-10 to 2024-01-15"})
	cw.Write([]string{"# 2024-02-07 to 2024-02-12"})
	cw.Write([]string{"# 2024-03-08 to 2024-03-13"})
	cw.Write([]string{"# 2024-04-05 to 2024-04-10"})
	cw.Flush()
}

// ─── Methodology ───────────────────────────────────────────────────────────

func handleMethodology(w http.ResponseWriter, r *http.Request) {
	pd := newPageData(r)
	user := getUserFromContext(r)
	if user != nil {
		pd.Data = map[string]interface{}{
			"CycleLength":  user.CycleLength,
			"PeriodLength": user.PeriodLength,
			"OvulationDay": user.CycleLength - 14,
			"FertileStart": user.CycleLength - 14 - 5,
			"FertileEnd":   user.CycleLength - 14 + 1,
		}
	}
	renderTemplate(w, r, "methodology", pd)
}
