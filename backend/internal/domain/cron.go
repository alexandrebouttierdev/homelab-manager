package domain

import (
	"errors"
	"fmt"
	"math/bits"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Bounds of the scheduled tasks module (SPEC_ADMINISTRATION.md § 8).
const (
	// CronNextRuns is the number of next runs shown for an entry.
	CronNextRuns = 5
	// MaxCronExpressionBytes bounds an expression typed in the inspector.
	MaxCronExpressionBytes = 200
	// cronHorizon bounds the search of the next runs: an expression that
	// never matches ("0 0 30 2 *") has none.
	cronHorizon = 5 * 366 * 24 * time.Hour
)

var ErrInvalidCronExpression = errors.New("invalid cron expression")

// CronSchedule is a parsed cron expression: five fields, or a macro.
type CronSchedule struct {
	Expression string
	// Reboot: @reboot, which runs at start-up and has no next run.
	Reboot   bool
	minutes  uint64
	hours    uint64
	days     uint64
	months   uint64
	weekdays uint64
	// daysStar and weekdaysStar follow Vixie cron: when both day fields
	// are restricted, a day matching either runs the task.
	daysStar     bool
	weekdaysStar bool
}

var cronMacros = map[string]string{
	"@yearly": "0 0 1 1 *", "@annually": "0 0 1 1 *", "@monthly": "0 0 1 * *",
	"@weekly": "0 0 * * 0", "@daily": "0 0 * * *", "@midnight": "0 0 * * *", "@hourly": "0 * * * *",
}

var (
	monthNames   = map[string]int{"jan": 1, "feb": 2, "mar": 3, "apr": 4, "may": 5, "jun": 6, "jul": 7, "aug": 8, "sep": 9, "oct": 10, "nov": 11, "dec": 12}
	weekdayNames = map[string]int{"sun": 0, "mon": 1, "tue": 2, "wed": 3, "thu": 4, "fri": 5, "sat": 6}
	cronFieldRE  = regexp.MustCompile(`^[0-9A-Za-z*/,\-]+$`)
)

type cronField struct {
	name     string
	min, max int
	names    map[string]int
}

var cronFields = []cronField{
	{"minute", 0, 59, nil}, {"hour", 0, 23, nil}, {"day of month", 1, 31, nil},
	{"month", 1, 12, monthNames}, {"day of week", 0, 7, weekdayNames},
}

// ParseCron parses a standard expression ("17 * * * *") or a macro
// ("@daily", "@reboot").
func ParseCron(expression string) (CronSchedule, error) {
	expression = strings.TrimSpace(expression)
	if expression == "" || len(expression) > MaxCronExpressionBytes {
		return CronSchedule{}, fmt.Errorf("%w: empty or too long", ErrInvalidCronExpression)
	}
	schedule := CronSchedule{Expression: expression}
	fields := strings.Fields(expression)
	if strings.HasPrefix(expression, "@") {
		macro := strings.ToLower(fields[0])
		if len(fields) != 1 {
			return CronSchedule{}, fmt.Errorf("%w: a macro stands alone", ErrInvalidCronExpression)
		}
		if macro == "@reboot" {
			schedule.Reboot = true
			return schedule, nil
		}
		expanded, ok := cronMacros[macro]
		if !ok {
			return CronSchedule{}, fmt.Errorf("%w: unknown macro %s", ErrInvalidCronExpression, fields[0])
		}
		fields = strings.Fields(expanded)
	}
	if len(fields) != 5 {
		return CronSchedule{}, fmt.Errorf("%w: five fields expected", ErrInvalidCronExpression)
	}
	sets := make([]uint64, 5)
	for i, field := range fields {
		set, err := parseCronField(field, cronFields[i])
		if err != nil {
			return CronSchedule{}, err
		}
		sets[i] = set
	}
	// Sunday is 0 or 7.
	if sets[4]&(1<<7) != 0 {
		sets[4] = sets[4]&^(1<<7) | 1
	}
	schedule.minutes, schedule.hours, schedule.days, schedule.months, schedule.weekdays = sets[0], sets[1], sets[2], sets[3], sets[4]
	schedule.daysStar, schedule.weekdaysStar = strings.HasPrefix(fields[2], "*"), strings.HasPrefix(fields[4], "*")
	return schedule, nil
}

func parseCronField(text string, field cronField) (uint64, error) {
	if !cronFieldRE.MatchString(text) {
		return 0, fmt.Errorf("%w: %s %q", ErrInvalidCronExpression, field.name, text)
	}
	var set uint64
	for _, item := range strings.Split(text, ",") {
		rangePart, stepPart, hasStep := strings.Cut(item, "/")
		step := 1
		if hasStep {
			value, err := strconv.Atoi(stepPart)
			if err != nil || value < 1 || value > field.max {
				return 0, fmt.Errorf("%w: %s step %q", ErrInvalidCronExpression, field.name, stepPart)
			}
			step = value
		}
		low, high := field.min, field.max
		switch {
		case rangePart == "*":
		case strings.Contains(rangePart, "-"):
			from, to, _ := strings.Cut(rangePart, "-")
			var err error
			if low, err = cronValue(from, field); err != nil {
				return 0, err
			}
			if high, err = cronValue(to, field); err != nil {
				return 0, err
			}
			if low > high {
				return 0, fmt.Errorf("%w: %s range %q", ErrInvalidCronExpression, field.name, rangePart)
			}
		default:
			value, err := cronValue(rangePart, field)
			if err != nil {
				return 0, err
			}
			low = value
			if !hasStep {
				high = value
			}
		}
		for value := low; value <= high; value += step {
			set |= 1 << value
		}
	}
	return set, nil
}

func cronValue(text string, field cronField) (int, error) {
	if value, ok := field.names[strings.ToLower(text)]; ok {
		return value, nil
	}
	value, err := strconv.Atoi(text)
	if err != nil || value < field.min || value > field.max {
		return 0, fmt.Errorf("%w: %s %q", ErrInvalidCronExpression, field.name, text)
	}
	return value, nil
}

func (s CronSchedule) dayMatches(t time.Time) bool {
	day := s.days&(1<<t.Day()) != 0
	weekday := s.weekdays&(1<<int(t.Weekday())) != 0
	if s.daysStar || s.weekdaysStar {
		return day && weekday
	}
	return day || weekday
}

// Next returns up to count runs strictly after the given time, in its
// location, within five years.
func (s CronSchedule) Next(after time.Time, count int) []time.Time {
	if s.Reboot || count <= 0 {
		return nil
	}
	loc := after.Location()
	limit := after.Add(cronHorizon)
	t := after.Truncate(time.Minute).Add(time.Minute)
	runs := make([]time.Time, 0, count)
	for len(runs) < count && t.Before(limit) {
		switch {
		case s.months&(1<<int(t.Month())) == 0:
			t = time.Date(t.Year(), t.Month()+1, 1, 0, 0, 0, 0, loc)
		case !s.dayMatches(t):
			t = time.Date(t.Year(), t.Month(), t.Day()+1, 0, 0, 0, 0, loc)
		case s.hours&(1<<t.Hour()) == 0:
			t = time.Date(t.Year(), t.Month(), t.Day(), t.Hour()+1, 0, 0, 0, loc)
		case s.minutes&(1<<t.Minute()) == 0:
			t = t.Add(time.Minute)
		default:
			runs = append(runs, t)
			t = t.Add(time.Minute)
		}
	}
	return runs
}

var frenchMonths = []string{"", "janvier", "février", "mars", "avril", "mai", "juin", "juillet", "août", "septembre", "octobre", "novembre", "décembre"}
var frenchWeekdays = []string{"dimanche", "lundi", "mardi", "mercredi", "jeudi", "vendredi", "samedi"}

func setValues(set uint64, max int) []int {
	values := make([]int, 0, bits.OnesCount64(set))
	for v := 0; v <= max; v++ {
		if set&(1<<v) != 0 {
			values = append(values, v)
		}
	}
	return values
}

func full(set uint64, min, max int) bool {
	for v := min; v <= max; v++ {
		if set&(1<<v) == 0 {
			return false
		}
	}
	return true
}

// uniformStep reports the step of a set that runs every step from 0 to
// max ("*/15"), 0 otherwise.
func uniformStep(set uint64, max int) int {
	values := setValues(set, max)
	if len(values) < 2 || values[0] != 0 {
		return 0
	}
	step := values[1] - values[0]
	for i := 1; i < len(values); i++ {
		if values[i]-values[i-1] != step {
			return 0
		}
	}
	if values[len(values)-1]+step <= max {
		return 0
	}
	return step
}

func frenchList(words []string) string {
	switch len(words) {
	case 0:
		return ""
	case 1:
		return words[0]
	}
	return strings.Join(words[:len(words)-1], ", ") + " et " + words[len(words)-1]
}

func dayOfMonth(day int) string {
	if day == 1 {
		return "1er"
	}
	return strconv.Itoa(day)
}

// Describe gives the schedule in French ("tous les jours à 03:00"), or a
// generic phrase for an expression that has no short wording.
func (s CronSchedule) Describe() string {
	if s.Reboot {
		return "Au démarrage de la machine"
	}
	minutes, hours := setValues(s.minutes, 59), setValues(s.hours, 23)
	allDays := full(s.days, 1, 31) && full(s.months, 1, 12) && full(s.weekdays, 0, 6)
	switch {
	case full(s.minutes, 0, 59) && full(s.hours, 0, 23) && allDays:
		return "Chaque minute"
	case uniformStep(s.minutes, 59) > 0 && full(s.hours, 0, 23) && allDays:
		return fmt.Sprintf("Toutes les %d minutes", uniformStep(s.minutes, 59))
	case len(minutes) == 1 && full(s.hours, 0, 23) && allDays:
		return fmt.Sprintf("Toutes les heures, à la minute %d", minutes[0])
	case len(minutes) == 1 && uniformStep(s.hours, 23) > 0 && allDays:
		return fmt.Sprintf("Toutes les %d heures, à la minute %d", uniformStep(s.hours, 23), minutes[0])
	}
	if len(minutes) != 1 || len(hours) == 0 || len(hours) > 6 {
		return "Selon l'expression « " + s.Expression + " »"
	}
	times := make([]string, 0, len(hours))
	for _, hour := range hours {
		times = append(times, fmt.Sprintf("%02d:%02d", hour, minutes[0]))
	}
	at := "à " + frenchList(times)
	days, months, weekdays := setValues(s.days, 31), setValues(s.months, 12), setValues(s.weekdays, 6)
	allMonths := full(s.months, 1, 12)
	switch {
	case allDays:
		return "Tous les jours " + at
	case allMonths && s.daysStar && !s.weekdaysStar:
		if full(s.weekdays, 1, 5) && len(weekdays) == 5 {
			return "Du lundi au vendredi " + at
		}
		names := make([]string, 0, len(weekdays))
		for _, day := range weekdays {
			names = append(names, frenchWeekdays[day])
		}
		return "Chaque " + frenchList(names) + " " + at
	case s.weekdaysStar && !s.daysStar && len(days) <= 3:
		numbers := make([]string, 0, len(days))
		for _, day := range days {
			numbers = append(numbers, dayOfMonth(day))
		}
		if allMonths {
			return "Le " + frenchList(numbers) + " de chaque mois " + at
		}
		if len(months) <= 3 {
			names := make([]string, 0, len(months))
			for _, month := range months {
				names = append(names, frenchMonths[month])
			}
			return "Le " + frenchList(numbers) + " " + frenchList(names) + " " + at
		}
	}
	return "Selon l'expression « " + s.Expression + " »"
}

// Bounds of the crontab inventory.
const (
	MaxCronEntries      = 1000
	MaxCronCommandBytes = 4096
	MaxCrontabBytes     = 64 << 10
)

// CronEntry is one line of a system crontab: the schedule, the account
// that runs it and its command, shown as is with secrets masked. Nothing
// is ever run.
type CronEntry struct {
	Source     string
	Line       int
	Expression string
	User       string
	Command    string
	// Error is set, and Schedule zero, when the expression is invalid.
	Error    string
	Schedule CronSchedule
}

// Valid reports whether the expression could be read.
func (e CronEntry) Valid() bool { return e.Error == "" }

// CronScriptDirectory is /etc/cron.hourly, daily, weekly or monthly: the
// scripts run-parts runs.
type CronScriptDirectory struct {
	Period  string
	Path    string
	Scripts []string
}

// CronInventory is what the system crontabs hold. Unreadable lists the
// files that could not be read.
type CronInventory struct {
	Entries     []CronEntry
	Directories []CronScriptDirectory
	// Anacron is set when /etc/anacrontab exists: anacron then runs the
	// daily, weekly and monthly scripts, catching up after a stop.
	Anacron    bool
	Unreadable []string
}

var secretPattern = regexp.MustCompile(`(?i)\b(password|passwd|pwd|token|secret|api[_-]?key)=("[^"]*"|'[^']*'|\S+)`)

// MaskCronSecrets hides the value of password=, token= and the like in a
// command (SPEC_ADMINISTRATION.md § 8).
func MaskCronSecrets(command string) string {
	return secretPattern.ReplaceAllString(command, "$1=••••")
}
