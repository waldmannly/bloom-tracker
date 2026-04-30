package main

import "time"

// WellnessTips holds phase-specific exercise and nutrition guidance
type WellnessTips struct {
	ExerciseTip      string
	ExerciseExamples []string
	NutritionTip     string
	KeyNutrients     []string
	FoodsToEat       []string
}

// CycleInfo holds calculated cycle state for a given day
type CycleInfo struct {
	Phase              string // "menstruation", "follicular", "ovulation", "luteal"
	CycleDay           int
	DaysUntilPeriod    int
	NextPeriodDate     time.Time
	IsInFertileWindow  bool
	FertileWindowStart time.Time
	FertileWindowEnd   time.Time
	OvulationDate      time.Time
	CurrentCycleStart  time.Time
	PhaseDescription   string
	PhaseEmoji         string
	PhaseColor         string
	PartnerTip         string
	Wellness           WellnessTips
	Encouragement      string
	PartnerActions     []string
	TreatIdeas         []string
}

// Prediction holds predicted dates for a future cycle
type Prediction struct {
	PeriodStart   time.Time
	PeriodEnd     time.Time
	OvulationDate time.Time
	FertileStart  time.Time
	FertileEnd    time.Time
}

// CalendarDay holds display data for one calendar cell
type CalendarDay struct {
	Date           time.Time
	Day            int
	IsToday        bool
	IsCurrentMonth bool
	IsPeriod       bool
	IsPredicted    bool
	IsOvulation    bool
	IsFertile      bool
	HasSymptoms    bool
	SymptomCount   int
	Phase          string
}

// CalendarMonth holds a full month of calendar data
type CalendarMonth struct {
	Year      int
	Month     int
	MonthName string
	Weeks     [][]CalendarDay
	PrevYear  int
	PrevMonth int
	NextYear  int
	NextMonth int
}

func midnight(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.Local)
}

func daysBetween(a, b time.Time) int {
	a, b = midnight(a), midnight(b)
	return int(b.Sub(a).Hours() / 24)
}

func calculateCycleInfo(lastPeriodStart time.Time, cycleLength, periodLength int, today time.Time) *CycleInfo {
	lastPeriodStart = midnight(lastPeriodStart)
	today = midnight(today)

	daysSince := daysBetween(lastPeriodStart, today)

	// Find the start of the current cycle
	currentCycleStart := lastPeriodStart
	if daysSince >= cycleLength {
		completed := daysSince / cycleLength
		currentCycleStart = lastPeriodStart.AddDate(0, 0, completed*cycleLength)
	}

	cycleDay := daysBetween(currentCycleStart, today) + 1

	// Ovulation typically occurs 14 days before the next period
	ovulationDay := cycleLength - 14
	if ovulationDay < periodLength+1 {
		ovulationDay = periodLength + 1
	}

	daysUntilPeriod := cycleLength - cycleDay + 1
	nextPeriodDate := today.AddDate(0, 0, daysUntilPeriod)

	// Determine phase
	var phase string
	switch {
	case cycleDay <= periodLength:
		phase = "menstruation"
	case cycleDay < ovulationDay-1:
		phase = "follicular"
	case cycleDay <= ovulationDay+1:
		phase = "ovulation"
	default:
		phase = "luteal"
	}

	// Fertile window: 5 days before ovulation through 1 day after
	fertileStart := currentCycleStart.AddDate(0, 0, ovulationDay-6)
	fertileEnd := currentCycleStart.AddDate(0, 0, ovulationDay)
	inFertile := !today.Before(fertileStart) && !today.After(fertileEnd)

	ovulationDate := currentCycleStart.AddDate(0, 0, ovulationDay-1)

	info := &CycleInfo{
		Phase:              phase,
		CycleDay:           cycleDay,
		DaysUntilPeriod:    daysUntilPeriod,
		NextPeriodDate:     nextPeriodDate,
		IsInFertileWindow:  inFertile,
		FertileWindowStart: fertileStart,
		FertileWindowEnd:   fertileEnd,
		OvulationDate:      ovulationDate,
		CurrentCycleStart:  currentCycleStart,
	}

	setPhaseInfo(info)
	setPartnerTip(info)
	setWellnessTips(info)
	setEncouragement(info)
	return info
}

func setPhaseInfo(info *CycleInfo) {
	switch info.Phase {
	case "menstruation":
		info.PhaseDescription = "Your period is here. Take it easy, stay hydrated, and listen to your body."
		info.PhaseEmoji = "🌺"
		info.PhaseColor = "menstruation"
	case "follicular":
		info.PhaseDescription = "Energy is rising! Your body is preparing for ovulation. Great time for new projects and activities."
		info.PhaseEmoji = "🌱"
		info.PhaseColor = "follicular"
	case "ovulation":
		info.PhaseDescription = "Peak energy and confidence! You may feel more social and creative. This is your fertile window."
		info.PhaseEmoji = "🌸"
		info.PhaseColor = "ovulation"
	case "luteal":
		info.PhaseDescription = "Winding down. You might crave comfort foods and need more rest. Be gentle with yourself."
		info.PhaseEmoji = "🌙"
		info.PhaseColor = "luteal"
	}
}

func setPartnerTip(info *CycleInfo) {
	switch info.Phase {
	case "menstruation":
		info.PartnerTip = "She may experience cramps, fatigue, and mood changes. Be extra supportive — bring her favorite snacks, a heating pad, or just check in on how she's feeling. Small gestures go a long way right now."
	case "follicular":
		info.PartnerTip = "Energy is on the rise! This is a great time for dates, activities, and trying new things together. She's likely feeling more social and adventurous."
	case "ovulation":
		info.PartnerTip = "Peak energy and confidence. She may feel extra social and attractive. If you're trying to conceive, this is the fertile window. If not, be mindful of protection."
	case "luteal":
		info.PartnerTip = "PMS symptoms may appear — mood swings, cravings, and fatigue are common. Be patient and understanding. A cozy night in, comfort food, or just listening goes a long way."
	}
}

func setWellnessTips(info *CycleInfo) {
	switch info.Phase {
	case "menstruation":
		info.Wellness = WellnessTips{
			ExerciseTip:      "Go easy on yourself — your body is doing important work. Gentle movement helps with cramps and mood.",
			ExerciseExamples: []string{"Walking", "Yoga", "Stretching", "Light swimming"},
			NutritionTip:     "Focus on iron-rich and anti-inflammatory foods to replenish what your body is losing.",
			KeyNutrients:     []string{"Iron", "Magnesium", "Omega-3", "Vitamin C"},
			FoodsToEat:       []string{"Spinach", "Lentils", "Dark chocolate", "Salmon", "Ginger tea", "Bone broth"},
		}
	case "follicular":
		info.Wellness = WellnessTips{
			ExerciseTip:      "Energy is building — your body recovers faster and builds muscle more easily right now.",
			ExerciseExamples: []string{"Strength training", "HIIT", "Running", "Dance classes", "Rock climbing"},
			NutritionTip:     "Fuel your rising energy with lean proteins and complex carbs. Great time to try new recipes.",
			KeyNutrients:     []string{"B Vitamins", "Zinc", "Vitamin E", "Probiotics"},
			FoodsToEat:       []string{"Eggs", "Avocado", "Fermented foods", "Lean chicken", "Quinoa", "Broccoli"},
		}
	case "ovulation":
		info.Wellness = WellnessTips{
			ExerciseTip:      "Peak performance time! You're strongest and have the most endurance this week.",
			ExerciseExamples: []string{"High intensity workouts", "Spin class", "Sprint intervals", "Group fitness", "Boxing"},
			NutritionTip:     "Support peak energy with antioxidant-rich whole foods and stay extra hydrated.",
			KeyNutrients:     []string{"Antioxidants", "B Vitamins", "Zinc", "Glutathione"},
			FoodsToEat:       []string{"Berries", "Bell peppers", "Raw veggies", "Nuts", "Seeds", "Coconut water"},
		}
	case "luteal":
		info.Wellness = WellnessTips{
			ExerciseTip:      "Gradually wind down intensity — your body is preparing. Listen to fatigue signals.",
			ExerciseExamples: []string{"Pilates", "Swimming", "Walking", "Light weights", "Restorative yoga"},
			NutritionTip:     "Combat cravings with complex carbs and magnesium. Don't restrict — nourish.",
			KeyNutrients:     []string{"Magnesium", "Calcium", "Vitamin B6", "Fiber"},
			FoodsToEat:       []string{"Sweet potatoes", "Brown rice", "Dark chocolate", "Bananas", "Oats", "Chamomile tea"},
		}
	}
}

func setEncouragement(info *CycleInfo) {
	switch info.Phase {
	case "menstruation":
		info.Encouragement = "You are doing amazing. Your body is powerful and resilient — rest is not laziness, it's recovery. This phase is temporary, and you're handling it beautifully. 💛"
		info.PartnerActions = []string{
			"Bring her a heating pad or hot water bottle",
			"Make her favorite warm drink (tea, cocoa, coffee)",
			"Pick up her favorite comfort snack without being asked",
			"Offer a gentle back or foot rub",
			"Handle dinner tonight — cook or order in",
			"Don't ask 'what's wrong' — just be present and kind",
			"Run a warm bath with some candles",
			"Let her pick the movie or show tonight",
		}
		info.TreatIdeas = []string{
			"🍫 Dark chocolate",
			"🛁 Warm bath bomb",
			"🌸 Fresh flowers",
			"☕ Her favorite latte",
			"🧦 Cozy fuzzy socks",
			"📖 New book or magazine",
		}
	case "follicular":
		info.Encouragement = "You're glowing with renewed energy! This is YOUR week — chase that goal, start that project, say yes to adventure. You've got this! ✨"
		info.PartnerActions = []string{
			"Plan a fun date — she has energy for something new!",
			"Suggest trying a new restaurant or recipe together",
			"Go for a hike, bike ride, or workout together",
			"Be her hype person — encourage her new ideas",
			"Start a project together you've been putting off",
		}
		info.TreatIdeas = []string{
			"🎨 Art supplies or craft kit",
			"🏔️ Day trip adventure",
			"🍳 Brunch date",
			"🎵 Concert or show tickets",
			"🧘 Yoga class together",
		}
	case "ovulation":
		info.Encouragement = "You're radiating confidence and strength! This is your peak — own it, celebrate it, and let yourself shine. You are magnetic right now! 🌟"
		info.PartnerActions = []string{
			"Compliment her — she's feeling herself and deserves it",
			"Plan a social outing — she's at her most social",
			"Be spontaneous — surprise her with a plan",
			"Take photos together — she'll love how she looks",
			"Have meaningful conversations — she's extra communicative",
		}
		info.TreatIdeas = []string{
			"💐 Surprise bouquet",
			"🍷 Nice dinner out",
			"💃 Dancing or fun night out",
			"📸 Do something photo-worthy together",
			"🎁 Small thoughtful gift",
		}
	case "luteal":
		info.Encouragement = "It's okay to slow down. You don't have to be productive every day to be valuable. Wrap yourself in comfort and grace — you deserve gentleness right now. 🌙"
		info.PartnerActions = []string{
			"Be patient with mood shifts — they're hormonal, not personal",
			"Suggest a cozy night in with her favorite show",
			"Don't comment on food choices — let her eat what she wants",
			"Give extra words of affirmation and reassurance",
			"Take something off her plate — do a chore she usually handles",
			"Listen without trying to fix — just validate her feelings",
		}
		info.TreatIdeas = []string{
			"🍕 Her favorite comfort food",
			"🕯️ Scented candle",
			"🧸 Something soft and cozy",
			"🎬 Movie marathon snacks",
			"🫖 Herbal tea sampler",
			"💆 Massage gift card",
		}
	}
}

func predictFuturePeriods(lastPeriodStart time.Time, cycleLength, periodLength, count int) []Prediction {
	lastPeriodStart = midnight(lastPeriodStart)
	today := midnight(time.Now())

	// Advance to the next predicted period after today
	currentStart := lastPeriodStart
	for !currentStart.After(today) {
		currentStart = currentStart.AddDate(0, 0, cycleLength)
	}

	ovulationDay := cycleLength - 14
	if ovulationDay < periodLength+1 {
		ovulationDay = periodLength + 1
	}

	predictions := make([]Prediction, 0, count)
	for i := 0; i < count; i++ {
		predictions = append(predictions, Prediction{
			PeriodStart:   currentStart,
			PeriodEnd:     currentStart.AddDate(0, 0, periodLength-1),
			OvulationDate: currentStart.AddDate(0, 0, ovulationDay-1),
			FertileStart:  currentStart.AddDate(0, 0, ovulationDay-6),
			FertileEnd:    currentStart.AddDate(0, 0, ovulationDay),
		})
		currentStart = currentStart.AddDate(0, 0, cycleLength)
	}
	return predictions
}

// generateCalendar builds the month grid with period/symptom/prediction overlays
func generateCalendar(year, month int, userID int64, cycleLength, periodLength int, lastPeriodStart *time.Time) *CalendarMonth {
	firstOfMonth := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.Local)
	lastOfMonth := firstOfMonth.AddDate(0, 1, -1)

	// Grid boundaries (full weeks)
	gridStart := firstOfMonth.AddDate(0, 0, -int(firstOfMonth.Weekday()))
	gridEnd := lastOfMonth.AddDate(0, 0, 6-int(lastOfMonth.Weekday()))

	// Load actual data
	periods, _ := getPeriodsInRange(userID, gridStart.AddDate(0, 0, -40), gridEnd)
	symptoms, _ := getSymptomsInRange(userID, gridStart, gridEnd)

	// Build period-day lookup
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

	// Build symptom count lookup
	symptomCounts := make(map[string]int)
	for _, s := range symptoms {
		symptomCounts[s.Date.Format("2006-01-02")]++
	}

	today := midnight(time.Now())

	var weeks [][]CalendarDay
	current := gridStart
	for !current.After(gridEnd) {
		var week []CalendarDay
		for i := 0; i < 7; i++ {
			key := current.Format("2006-01-02")
			day := CalendarDay{
				Date:           current,
				Day:            current.Day(),
				IsToday:        current.Equal(today),
				IsCurrentMonth: current.Month() == time.Month(month),
				IsPeriod:       periodDays[key],
				HasSymptoms:    symptomCounts[key] > 0,
				SymptomCount:   symptomCounts[key],
			}

			if lastPeriodStart != nil {
				info := calculateCycleInfo(*lastPeriodStart, cycleLength, periodLength, current)
				day.Phase = info.Phase
				if info.Phase == "menstruation" && !day.IsPeriod && current.After(today) {
					day.IsPredicted = true
				}
				if info.Phase == "ovulation" {
					day.IsOvulation = true
				}
				if info.IsInFertileWindow {
					day.IsFertile = true
				}
			}

			week = append(week, day)
			current = current.AddDate(0, 0, 1)
		}
		weeks = append(weeks, week)
	}

	prev := firstOfMonth.AddDate(0, -1, 0)
	next := firstOfMonth.AddDate(0, 1, 0)

	return &CalendarMonth{
		Year:      year,
		Month:     month,
		MonthName: firstOfMonth.Format("January 2006"),
		Weeks:     weeks,
		PrevYear:  prev.Year(),
		PrevMonth: int(prev.Month()),
		NextYear:  next.Year(),
		NextMonth: int(next.Month()),
	}
}
