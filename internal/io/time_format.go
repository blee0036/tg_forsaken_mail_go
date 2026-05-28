package io

import (
	"strings"
	"time"
)

// IsZhLang checks if the given language code belongs to the zh (Chinese) family.
// It performs a case-insensitive prefix match on "zh", covering zh, zh-Hans, zh-Hant, zh_CN, zh-TW, etc.
func IsZhLang(lang string) bool {
	lower := strings.ToLower(strings.TrimSpace(lang))
	if lower == "zh" {
		return true
	}
	// Check for zh- or zh_ prefix (e.g., zh-Hans, zh-Hant, zh_CN, zh-TW)
	if strings.HasPrefix(lower, "zh-") || strings.HasPrefix(lower, "zh_") {
		return true
	}
	return false
}

// FormatMailTime formats a mail time for display based on user language preference.
//   - dateTime != nil: convert to user timezone (zh series → UTC+8, others → UTC+0) and format
//   - dateTime == nil && rawDate != "": return rawDate as-is (unparseable fallback, no timezone conversion)
//   - dateTime == nil && rawDate == "": use now parameter, format with user timezone
//
// Output format includes timezone identifier: "2006-01-02 15:04:05 +08:00" or "2006-01-02 15:04:05 +00:00"
func FormatMailTime(dateTime *time.Time, rawDate string, lang string, now time.Time) string {
	const layout = "2006-01-02 15:04:05 -07:00"

	if dateTime != nil {
		return formatInUserTimezone(*dateTime, lang, layout)
	}

	if rawDate != "" {
		return rawDate
	}

	return formatInUserTimezone(now, lang, layout)
}

// formatInUserTimezone converts a time to the user's timezone and formats it.
// zh series languages use UTC+8, all others use UTC+0.
func formatInUserTimezone(t time.Time, lang string, layout string) string {
	if IsZhLang(lang) {
		loc := time.FixedZone("UTC+8", 8*60*60)
		return t.In(loc).Format(layout)
	}
	return t.UTC().Format(layout)
}
