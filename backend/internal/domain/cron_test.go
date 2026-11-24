package domain

import (
	"errors"
	"testing"
	"time"
)

func mustCron(t *testing.T, expression string) CronSchedule {
	t.Helper()
	schedule, err := ParseCron(expression)
	if err != nil {
		t.Fatalf("ParseCron(%q) = %v", expression, err)
	}
	return schedule
}

func TestParseCronRefusesInvalidExpressions(t *testing.T) {
	for _, expression := range []string{
		"", "* * * *", "* * * * * *", "60 * * * *", "* 24 * * *", "* * 0 * *", "* * * 13 *", "* * * * 8",
		"*/0 * * * *", "5-1 * * * *", "@every 5m", "@daily extra", "a * * * *", "1;2 * * * *",
	} {
		if _, err := ParseCron(expression); !errors.Is(err, ErrInvalidCronExpression) {
			t.Errorf("%q accepted (%v)", expression, err)
		}
	}
}

func TestCronNext(t *testing.T) {
	paris, err := time.LoadLocation("Europe/Paris")
	if err != nil {
		t.Skip("no time zone database")
	}
	at := time.Date(2026, 11, 24, 10, 7, 30, 0, paris)
	cases := []struct {
		expression string
		want       []time.Time
	}{
		{"*/15 * * * *", []time.Time{
			time.Date(2026, 11, 24, 10, 15, 0, 0, paris), time.Date(2026, 11, 24, 10, 30, 0, 0, paris),
		}},
		{"17 * * * *", []time.Time{
			time.Date(2026, 11, 24, 10, 17, 0, 0, paris), time.Date(2026, 11, 24, 11, 17, 0, 0, paris),
		}},
		{"@daily", []time.Time{
			time.Date(2026, 11, 25, 0, 0, 0, 0, paris), time.Date(2026, 11, 26, 0, 0, 0, 0, paris),
		}},
		{"30 4 * * mon-fri", []time.Time{
			time.Date(2026, 11, 25, 4, 30, 0, 0, paris), time.Date(2026, 11, 26, 4, 30, 0, 0, paris),
		}},
		{"0 6 1 jan,jul *", []time.Time{
			time.Date(2027, 1, 1, 6, 0, 0, 0, paris), time.Date(2027, 7, 1, 6, 0, 0, 0, paris),
		}},
		// Both day fields restricted: the 1st of the month or any Sunday.
		{"0 3 1 * 0", []time.Time{
			time.Date(2026, 11, 29, 3, 0, 0, 0, paris), time.Date(2026, 12, 1, 3, 0, 0, 0, paris),
		}},
		// Sunday written 7.
		{"0 3 * * 7", []time.Time{time.Date(2026, 11, 29, 3, 0, 0, 0, paris)}},
	}
	for _, tc := range cases {
		got := mustCron(t, tc.expression).Next(at, len(tc.want))
		if len(got) != len(tc.want) {
			t.Errorf("%q: %v, want %v", tc.expression, got, tc.want)
			continue
		}
		for i := range got {
			if !got[i].Equal(tc.want[i]) {
				t.Errorf("%q run %d = %v, want %v", tc.expression, i, got[i], tc.want[i])
			}
		}
	}
	if runs := mustCron(t, "0 0 30 2 *").Next(at, 3); len(runs) != 0 {
		t.Errorf("February 30 = %v, want none", runs)
	}
	if runs := mustCron(t, "@reboot").Next(at, 3); len(runs) != 0 {
		t.Errorf("@reboot = %v, want none", runs)
	}
	// The night of the switch to summer time, 02:30 does not exist.
	if runs := mustCron(t, "30 2 * * *").Next(time.Date(2027, 3, 27, 12, 0, 0, 0, paris), 2); len(runs) != 2 {
		t.Fatalf("summer time = %v", runs)
	}
}

func TestCronDescribe(t *testing.T) {
	cases := map[string]string{
		"* * * * *":             "Chaque minute",
		"*/5 * * * *":           "Toutes les 5 minutes",
		"17 * * * *":            "Toutes les heures, à la minute 17",
		"0 */6 * * *":           "Toutes les 6 heures, à la minute 0",
		"25 6 * * *":            "Tous les jours à 06:25",
		"0 6,18 * * *":          "Tous les jours à 06:00 et 18:00",
		"47 6 * * 7":            "Chaque dimanche à 06:47",
		"0 9 * * 1,3":           "Chaque lundi et mercredi à 09:00",
		"0 9 * * 1-5":           "Du lundi au vendredi à 09:00",
		"52 6 1 * *":            "Le 1er de chaque mois à 06:52",
		"0 0 1 1 *":             "Le 1er janvier à 00:00",
		"@reboot":               "Au démarrage de la machine",
		"@weekly":               "Chaque dimanche à 00:00",
		"1,2,3,4,5,6,7 * * * *": "Selon l'expression « 1,2,3,4,5,6,7 * * * * »",
	}
	for expression, want := range cases {
		if got := mustCron(t, expression).Describe(); got != want {
			t.Errorf("%q = %q, want %q", expression, got, want)
		}
	}
}
