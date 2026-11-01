package advisor

import (
	"fmt"
	"math"
	"time"
)

func round1(f float64) float64 { return math.Round(f*10) / 10 }

func durationDays(d time.Duration) string { return fmt.Sprintf("%.1f", d.Hours()/24) }

func sprintfDays(format string, days float64) string {
	return fmt.Sprintf(format, fmt.Sprintf("%.1f", days))
}
