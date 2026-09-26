package provider

import "time"

func testNow() time.Time {
	return time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)
}
