package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/smtp"
	"strings"
	"time"
)

// EmailConfig holds email delivery settings (SMTP or HTTP API)
type EmailConfig struct {
	// SMTP mode
	SMTPHost    string
	SMTPPort    string
	SenderEmail string
	SenderPass  string
	// HTTP API mode (e.g. a local email microservice)
	APIURL string
	APIKey string
	// State
	Mode    string // "smtp", "api", or ""
	Enabled bool
}

var emailConfig EmailConfig

func initEmail(host, port, email, pass, apiURL, apiKey string) {
	emailConfig = EmailConfig{
		SMTPHost:    host,
		SMTPPort:    port,
		SenderEmail: email,
		SenderPass:  pass,
		APIURL:      apiURL,
		APIKey:      apiKey,
	}

	if apiURL != "" && apiKey != "" {
		emailConfig.Mode = "api"
		emailConfig.Enabled = true
		log.Printf("📧 Email notifications enabled via API (%s)", apiURL)
	} else if email != "" && pass != "" {
		emailConfig.Mode = "smtp"
		emailConfig.Enabled = true
		log.Printf("📧 Email notifications enabled via SMTP (%s)", host)
	} else {
		log.Println("📧 Email notifications disabled (set EMAIL_API_URL+EMAIL_API_KEY or SMTP_EMAIL+SMTP_PASS)")
	}
}

func sanitizeEmailHeader(s string) string {
	s = strings.ReplaceAll(s, "\r", "")
	s = strings.ReplaceAll(s, "\n", "")
	return s
}

func sendEmail(to, subject, body string) error {
	if !emailConfig.Enabled {
		log.Printf("📧 [DRY RUN] Would email %s: %s", to, subject)
		return nil
	}

	switch emailConfig.Mode {
	case "api":
		return sendEmailViaAPI(to, subject, body)
	case "smtp":
		return sendEmailViaSMTP(to, subject, body)
	default:
		return fmt.Errorf("no email backend configured")
	}
}

func sendEmailViaAPI(to, subject, htmlBody string) error {
	payload := map[string]string{
		"to":      to,
		"subject": subject,
		"html":    htmlBody,
	}
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshaling email payload: %w", err)
	}

	req, err := http.NewRequest("POST", emailConfig.APIURL, bytes.NewReader(jsonData))
	if err != nil {
		return fmt.Errorf("creating email request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", emailConfig.APIKey)

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("📧 Email API error to %s: %v", to, err)
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		respBody, _ := io.ReadAll(resp.Body)
		log.Printf("📧 Email API error to %s: status %d — %s", to, resp.StatusCode, string(respBody))
		return fmt.Errorf("email API returned %d", resp.StatusCode)
	}

	log.Printf("📧 Email sent to %s: %s (via API)", to, subject)
	return nil
}

func sendEmailViaSMTP(to, subject, body string) error {
	from := emailConfig.SenderEmail
	msg := fmt.Sprintf(
		"From: Bloom Period Tracker <%s>\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/html; charset=utf-8\r\n\r\n%s",
		from, sanitizeEmailHeader(to), sanitizeEmailHeader(subject), body,
	)

	auth := smtp.PlainAuth("", from, emailConfig.SenderPass, emailConfig.SMTPHost)
	addr := emailConfig.SMTPHost + ":" + emailConfig.SMTPPort

	if err := smtp.SendMail(addr, auth, from, []string{to}, []byte(msg)); err != nil {
		log.Printf("📧 Email send error to %s: %v", to, err)
		return err
	}
	log.Printf("📧 Email sent to %s: %s (via SMTP)", to, subject)
	return nil
}

// ─── Phase Notification Emails ──────────────────────────────────────────

func buildPhaseEmail(ownerName, phase string, info *CycleInfo, customPrefs string) (string, string) {
	var subject string
	switch phase {
	case "menstruation":
		subject = fmt.Sprintf("🌺 %s's period has started — here's how to help", ownerName)
	case "follicular":
		subject = fmt.Sprintf("🌱 %s is feeling energized — great time for plans!", ownerName)
	case "ovulation":
		subject = fmt.Sprintf("🌸 %s is in peak form — make it count!", ownerName)
	case "luteal":
		subject = fmt.Sprintf("🌙 %s needs extra love right now", ownerName)
	}

	// Build action items HTML
	actionsHTML := ""
	for _, a := range info.PartnerActions {
		actionsHTML += fmt.Sprintf(`<li style="padding:6px 0;color:#555;">%s</li>`, a)
	}

	// Build treat ideas HTML
	treatsHTML := ""
	for _, t := range info.TreatIdeas {
		treatsHTML += fmt.Sprintf(`<span style="display:inline-block;background:#FFF9F0;padding:6px 14px;border-radius:20px;margin:4px;font-size:13px;color:#b07840;">%s</span>`, t)
	}

	// Build foods HTML
	foodsHTML := ""
	for _, f := range info.Wellness.FoodsToEat {
		foodsHTML += fmt.Sprintf(`<span style="display:inline-block;background:#F0FAF5;padding:6px 14px;border-radius:20px;margin:4px;font-size:13px;color:#4a8a87;">%s</span>`, f)
	}

	// Build activities HTML
	activitiesHTML := ""
	for _, e := range info.Wellness.ExerciseExamples {
		activitiesHTML += fmt.Sprintf(`<span style="display:inline-block;background:#F0F0FA;padding:6px 14px;border-radius:20px;margin:4px;font-size:13px;color:#6b4f8a;">%s</span>`, e)
	}

	// Build custom preferences HTML
	customPrefsHTML := ""
	if strings.TrimSpace(customPrefs) != "" {
		items := strings.FieldsFunc(customPrefs, func(r rune) bool { return r == ',' || r == '\n' })
		itemsHTML := ""
		for _, item := range items {
			item = strings.TrimSpace(item)
			if item != "" {
				itemsHTML += fmt.Sprintf(`<li style="padding:6px 0;color:#555;">%s</li>`, item)
			}
		}
		if itemsHTML != "" {
			customPrefsHTML = fmt.Sprintf(`
  <div style="background:#fff;border-radius:12px;padding:20px;box-shadow:0 2px 12px rgba(100,50,60,0.06);margin-bottom:16px;border-left:4px solid #D4788C;">
    <h2 style="font-size:16px;margin:0 0 8px;color:#D4788C;">💛 What %s Actually Wants Right Now</h2>
    <ul style="list-style:none;padding:0;margin:0;">%s</ul>
  </div>`, ownerName, itemsHTML)
		}
	}

	fertileStatus := "Not in fertile window"
	fertileColor := "#A85566"
	if info.IsInFertileWindow {
		fertileStatus = "🌿 Fertile"
		fertileColor = "#82B366"
	}

	body := fmt.Sprintf(`<!DOCTYPE html>
<html><head><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1.0"></head>
<body style="margin:0;padding:0;background:#FFF9F9;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,sans-serif;">
<div style="max-width:520px;margin:0 auto;padding:24px;">

  <div style="text-align:center;padding:32px 24px;border-radius:16px;background:%s;margin-bottom:20px;">
    <div style="font-size:48px;margin-bottom:4px;">%s</div>
    <h1 style="font-size:22px;margin:0 0 4px;color:%s;">%s Phase</h1>
    <p style="margin:0;font-size:14px;color:%s;">Day %d of %s's cycle</p>
  </div>

  <div style="background:#fff;border-radius:12px;padding:20px;box-shadow:0 2px 12px rgba(100,50,60,0.06);margin-bottom:16px;border-left:4px solid #E8A87C;">
    <h2 style="font-size:16px;margin:0 0 8px;color:#E8A87C;">💡 What She Needs From You</h2>
    <p style="color:#555;line-height:1.7;margin:0 0 12px;">%s</p>
    <ul style="list-style:none;padding:0;margin:0;">%s</ul>
  </div>

  %s

  <div style="background:#fff;border-radius:12px;padding:20px;box-shadow:0 2px 12px rgba(100,50,60,0.06);margin-bottom:16px;">
    <h2 style="font-size:16px;margin:0 0 12px;color:#b07840;">🎁 Treat Ideas</h2>
    <div>%s</div>
  </div>

  <div style="background:#fff;border-radius:12px;padding:20px;box-shadow:0 2px 12px rgba(100,50,60,0.06);margin-bottom:16px;">
    <h2 style="font-size:16px;margin:0 0 12px;color:#4a8a87;">🥗 Foods That Help Right Now</h2>
    <div>%s</div>
  </div>

  <div style="background:#fff;border-radius:12px;padding:20px;box-shadow:0 2px 12px rgba(100,50,60,0.06);margin-bottom:16px;">
    <h2 style="font-size:16px;margin:0 0 12px;color:#6b4f8a;">🏃‍♀️ Good Activities Together</h2>
    <div>%s</div>
  </div>

  <div style="background:linear-gradient(135deg,#FFF9F9,#FFF0F3);border-radius:12px;padding:20px;margin-bottom:16px;text-align:center;border:1px solid #F2D1D9;">
    <p style="font-size:15px;color:#A85566;line-height:1.7;margin:0;font-style:italic;">"%s"</p>
    <p style="font-size:12px;color:#ccc;margin:8px 0 0;">— Share this with her 💛</p>
  </div>

  <div style="display:flex;gap:12px;margin-bottom:20px;">
    <div style="flex:1;background:#fff;border-radius:10px;padding:14px;text-align:center;box-shadow:0 2px 12px rgba(100,50,60,0.06);">
      <div style="font-size:20px;font-weight:700;color:#A85566;">%d</div>
      <div style="font-size:11px;color:#A0A0A0;text-transform:uppercase;">days until period</div>
    </div>
    <div style="flex:1;background:#fff;border-radius:10px;padding:14px;text-align:center;box-shadow:0 2px 12px rgba(100,50,60,0.06);">
      <div style="font-size:14px;font-weight:600;color:%s;">%s</div>
      <div style="font-size:11px;color:#A0A0A0;text-transform:uppercase;">fertility</div>
    </div>
  </div>

  <p style="text-align:center;font-size:12px;color:#A0A0A0;margin:24px 0 0;">
    🌸 Bloom — Sent with love, not ads. No tracking. No BS.
  </p>

</div></body></html>`,
		phaseEmailBG(phase), info.PhaseEmoji, phaseEmailColor(phase),
		strings.ToUpper(phase[:1])+phase[1:],
		phaseEmailColor(phase), info.CycleDay, ownerName,
		info.PartnerTip, actionsHTML,
		customPrefsHTML,
		treatsHTML, foodsHTML, activitiesHTML,
		info.Encouragement,
		info.DaysUntilPeriod, fertileColor, fertileStatus,
	)

	return subject, body
}

func phaseEmailBG(phase string) string {
	switch phase {
	case "menstruation":
		return "#FFF0F3"
	case "follicular":
		return "#F0FAF9"
	case "ovulation":
		return "#FFF8F0"
	case "luteal":
		return "#F8F0FF"
	}
	return "#FFF9F9"
}

func phaseEmailColor(phase string) string {
	switch phase {
	case "menstruation":
		return "#D4788C"
	case "follicular":
		return "#4a8a87"
	case "ovulation":
		return "#b07840"
	case "luteal":
		return "#6b4f8a"
	}
	return "#2D2D2D"
}

// notifyPartnerIfPhaseChangedchecks if the cycle phase has changed and emails the partner
func notifyPartnerIfPhaseChanged(ownerID int64) {
	owner, err := getUserByID(ownerID)
	if err != nil {
		return
	}

	partner, err := getPartnerForOwner(ownerID)
	if err != nil || !getPartnerNotify(ownerID) {
		return
	}

	lastPeriod, err := getLastPeriod(ownerID)
	if err != nil {
		return
	}

	today := midnight(time.Now())
	info := calculateCycleInfo(lastPeriod.StartDate, owner.CycleLength, owner.PeriodLength, today)

	// Check if we already notified for this phase today
	lastNotified := getLastNotifiedPhase(ownerID)
	phaseKey := fmt.Sprintf("%s|%s", info.Phase, today.Format("2006-01-02"))
	if lastNotified == phaseKey {
		return
	}

	phasePrefs := getPhasePreferences(ownerID)
	subject, body := buildPhaseEmail(owner.Name, info.Phase, info, phasePrefs[info.Phase])

	go func() {
		if err := sendEmail(partner.Email, subject, body); err == nil {
			setLastNotifiedPhase(ownerID, phaseKey)
		}
	}()
}

// notifyPartnerOfSymptoms alerts the partner when significant symptoms are logged
func notifyPartnerOfSymptoms(ownerID int64, symptomName, category string, severity int) {
	if severity < 4 {
		return // Only notify for severe symptoms (4-5)
	}

	owner, err := getUserByID(ownerID)
	if err != nil {
		return
	}

	partner, err := getPartnerForOwner(ownerID)
	if err != nil || !getPartnerNotify(ownerID) {
		return
	}

	var emoji string
	switch category {
	case "physical":
		emoji = "🤕"
	case "emotional":
		emoji = "💙"
	case "flow":
		emoji = "🩸"
	default:
		emoji = "📝"
	}

	subject := fmt.Sprintf("%s %s is feeling %s (severity %d/5)", emoji, owner.Name, symptomName, severity)

	// Get current phase for context
	lastPeriod, err := getLastPeriod(ownerID)
	phaseInfo := ""
	treatSection := ""
	if err == nil {
		info := calculateCycleInfo(lastPeriod.StartDate, owner.CycleLength, owner.PeriodLength, time.Now())
		phaseInfo = fmt.Sprintf(`<p style="margin:12px 0 0;font-size:13px;color:#999;">Currently in %s phase (Day %d)</p>`, info.Phase, info.CycleDay)

		treatsHTML := ""
		for _, t := range info.TreatIdeas {
			treatsHTML += fmt.Sprintf(`<span style="display:inline-block;background:#FFF9F0;padding:6px 14px;border-radius:20px;margin:4px;font-size:13px;color:#b07840;">%s</span>`, t)
		}
		treatSection = fmt.Sprintf(`
		<div style="background:#fff;border-radius:12px;padding:16px;margin-top:16px;border:1px solid #eee;">
			<h3 style="font-size:14px;margin:0 0 8px;color:#b07840;">🎁 Maybe bring her...</h3>
			<div>%s</div>
		</div>`, treatsHTML)
	}

	sevDots := ""
	for i := 1; i <= 5; i++ {
		if i <= severity {
			sevDots += `<span style="color:#D4788C;">●</span> `
		} else {
			sevDots += `<span style="color:#EDE5E5;">●</span> `
		}
	}

	body := fmt.Sprintf(`<!DOCTYPE html>
<html><head><meta charset="UTF-8"></head>
<body style="margin:0;padding:0;background:#FFF9F9;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,sans-serif;">
<div style="max-width:480px;margin:0 auto;padding:24px;">

  <div style="text-align:center;padding:24px;border-radius:16px;background:#FFF0F3;margin-bottom:16px;">
    <div style="font-size:40px;margin-bottom:4px;">%s</div>
    <h2 style="font-size:18px;margin:0;color:#A85566;">%s logged a symptom</h2>
  </div>

  <div style="background:#fff;border-radius:12px;padding:20px;box-shadow:0 2px 12px rgba(100,50,60,0.06);margin-bottom:16px;">
    <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:8px;">
      <span style="font-weight:700;font-size:16px;color:#2D2D2D;">%s</span>
      <span style="font-size:14px;">%s</span>
    </div>
    <div style="display:flex;justify-content:space-between;align-items:center;">
      <span style="font-size:13px;padding:4px 12px;background:%s;color:%s;border-radius:20px;">%s</span>
      <span style="font-size:11px;color:#A0A0A0;">Severity %d/5</span>
    </div>
    %s
  </div>

  <div style="background:linear-gradient(135deg,#FFF9F9,#FFF0F3);border-radius:12px;padding:16px;text-align:center;margin-bottom:16px;border:1px solid #F2D1D9;">
    <p style="font-size:14px;color:#A85566;margin:0;">💛 A small gesture right now would mean the world.</p>
  </div>

  %s

  <p style="text-align:center;font-size:12px;color:#A0A0A0;margin:20px 0 0;">🌸 Bloom — Sent with love, not ads.</p>
</div></body></html>`,
		emoji, owner.Name,
		symptomName, sevDots,
		categoryBG(category), categoryColor(category), category, severity,
		phaseInfo,
		treatSection,
	)

	go sendEmail(partner.Email, subject, body)
}

func categoryBG(cat string) string {
	switch cat {
	case "physical":
		return "#FFF0F3"
	case "emotional":
		return "#F8F0FF"
	case "flow":
		return "#fef2f2"
	default:
		return "#FFF8F0"
	}
}

func categoryColor(cat string) string {
	switch cat {
	case "physical":
		return "#D4788C"
	case "emotional":
		return "#9B7EBD"
	case "flow":
		return "#c53030"
	default:
		return "#E8A87C"
	}
}

// ─── Notification state (stored in DB) ──────────────────────────────────

func getPartnerNotify(ownerID int64) bool {
	var enabled bool
	err := db.QueryRow("SELECT partner_notify FROM users WHERE id = ?", ownerID).Scan(&enabled)
	return err == nil && enabled
}

func setPartnerNotify(ownerID int64, enabled bool) error {
	_, err := db.Exec("UPDATE users SET partner_notify = ? WHERE id = ?", enabled, ownerID)
	return err
}

func getLastNotifiedPhase(ownerID int64) string {
	var phase string
	db.QueryRow("SELECT COALESCE(last_notified_phase, '') FROM users WHERE id = ?", ownerID).Scan(&phase)
	return phase
}

func setLastNotifiedPhase(ownerID int64, phase string) {
	db.Exec("UPDATE users SET last_notified_phase = ? WHERE id = ?", phase, ownerID)
}

// startWeeklyNotifier runs a background goroutine that sends partner summary emails once per week (Monday 8 AM).
// Event-driven emails (phase changes, severe symptoms) still send immediately.
func startWeeklyNotifier() {
	if !emailConfig.Enabled {
		return
	}
	log.Println("📧 Weekly partner email notifier started")

	go func() {
		for {
			now := time.Now()
			// Find next Monday at 8 AM
			daysUntilMonday := (8 - int(now.Weekday())) % 7
			if daysUntilMonday == 0 && now.Hour() >= 8 {
				daysUntilMonday = 7
			}
			next := time.Date(now.Year(), now.Month(), now.Day()+daysUntilMonday, 8, 0, 0, 0, time.Local)
			log.Printf("📧 Next weekly partner email at %s", next.Format("Mon, Jan 2 3:04 PM"))
			time.Sleep(next.Sub(now))
			sendWeeklyPartnerNotifications()
		}
	}()
}

func sendWeeklyPartnerNotifications() {
	pairs, err := getOwnersWithNotifyPartners()
	if err != nil {
		log.Printf("📧 Error fetching notification pairs: %v", err)
		return
	}

	today := midnight(time.Now())
	for _, pair := range pairs {
		lastPeriod, err := getLastPeriod(pair.OwnerID)
		if err != nil {
			continue
		}

		info := calculateCycleInfo(lastPeriod.StartDate, pair.CycleLength, pair.PeriodLength, today)
		phasePrefs := getPhasePreferences(pair.OwnerID)
		subject, body := buildPhaseEmail(pair.OwnerName, info.Phase, info, phasePrefs[info.Phase])
		subject = "📅 Weekly Update: " + subject

		if err := sendEmail(pair.PartnerEmail, subject, body); err != nil {
			continue
		}

		logNotification(pair.PartnerID, "email", info.Phase)
		log.Printf("📧 Weekly update sent to %s (%s)", pair.PartnerName, pair.PartnerEmail)
	}
}
