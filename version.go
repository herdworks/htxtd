package htxtd

import (
	"time"
)

var (
	LibName           = "htxtd"
	LibVersion        = "v0.2.0"
	LibReleaseDate, _ = time.Parse(time.RFC3339, "2026-08-20T13:00:00-07:00") // Usage: LibReleaseDate.Format(time.RFC3339)
)
