package timezone

import (
	"testing"
	"time"
)

func TestCalendarAndFixedOffsetContracts(t *testing.T) {
	for _, sample := range []struct {
		date     string
		shanghai int
	}{{"2026-10-22", 8}, {"1990-07-01", 9}} {
		instant, err := time.Parse("2006-01-02", sample.date)
		if err != nil {
			t.Fatal(err)
		}
		_, historical := instant.In(Shanghai).Zone()
		_, fixed := instant.In(Beijing).Zone()
		if historical != sample.shanghai*3600 || fixed != 8*3600 {
			t.Fatalf("%s: calendar=%d fixed=%d", sample.date, historical, fixed)
		}
	}
}
