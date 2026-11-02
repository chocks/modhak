package advisor

import (
	"fmt"
	"math"
	"time"
)

func round1(f float64) float64 { return math.Round(f*10) / 10 }

func durationDays(d time.Duration) string { return fmt.Sprintf("%.1f", d.Hours()/24) }

// humanDuration renders an observation window in a unit that reads well:
// minutes for a fresh server, hours up to two days, then days.
func humanDuration(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "less than a minute"
	case d < 2*time.Minute:
		return "1 minute"
	case d < 2*time.Hour:
		return fmt.Sprintf("%d minutes", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%.1f hours", d.Hours())
	default:
		return fmt.Sprintf("%.1f days", d.Hours()/24)
	}
}
