package io

import (
	"testing"
	"time"
)

func TestIsZhLang(t *testing.T) {
	tests := []struct {
		lang     string
		expected bool
	}{
		{"zh", true},
		{"zh-Hans", true},
		{"zh-Hant", true},
		{"zh_CN", true},
		{"zh-TW", true},
		{"zh-HK", true},
		{"Zh", true},
		{"ZH", true},
		{"ZH-HANS", true},
		{"ZH_CN", true},
		{"en", false},
		{"en-US", false},
		{"ja", false},
		{"ko", false},
		{"", false},
		{"  zh  ", true},
		{"zho", false},
		{"zha", false},
	}

	for _, tt := range tests {
		t.Run(tt.lang, func(t *testing.T) {
			result := IsZhLang(tt.lang)
			if result != tt.expected {
				t.Errorf("IsZhLang(%q) = %v, want %v", tt.lang, result, tt.expected)
			}
		})
	}
}

func TestFormatMailTime_DateTimeNotNil(t *testing.T) {
	// 2024-06-15 12:00:00 UTC
	dt := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)

	// zh language should show UTC+8
	result := FormatMailTime(&dt, "", "zh", time.Now())
	expected := "2024-06-15 20:00:00 +08:00"
	if result != expected {
		t.Errorf("FormatMailTime(zh) = %q, want %q", result, expected)
	}

	// en language should show UTC+0
	result = FormatMailTime(&dt, "", "en", time.Now())
	expected = "2024-06-15 12:00:00 +00:00"
	if result != expected {
		t.Errorf("FormatMailTime(en) = %q, want %q", result, expected)
	}

	// zh-Hans should show UTC+8
	result = FormatMailTime(&dt, "", "zh-Hans", time.Now())
	expected = "2024-06-15 20:00:00 +08:00"
	if result != expected {
		t.Errorf("FormatMailTime(zh-Hans) = %q, want %q", result, expected)
	}
}

func TestFormatMailTime_DateTimeNilRawDateNonEmpty(t *testing.T) {
	rawDate := "Mon, 15 Jun 2024 12:00:00 +0000 (some weird format)"
	result := FormatMailTime(nil, rawDate, "zh", time.Now())
	if result != rawDate {
		t.Errorf("FormatMailTime(nil, rawDate) = %q, want %q", result, rawDate)
	}
}

func TestFormatMailTime_DateTimeNilRawDateEmpty(t *testing.T) {
	now := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)

	// zh language should format now as UTC+8
	result := FormatMailTime(nil, "", "zh", now)
	expected := "2024-06-15 20:00:00 +08:00"
	if result != expected {
		t.Errorf("FormatMailTime(nil, empty, zh) = %q, want %q", result, expected)
	}

	// en language should format now as UTC+0
	result = FormatMailTime(nil, "", "en", now)
	expected = "2024-06-15 12:00:00 +00:00"
	if result != expected {
		t.Errorf("FormatMailTime(nil, empty, en) = %q, want %q", result, expected)
	}
}

func TestFormatMailTime_TimezoneIdentifier(t *testing.T) {
	dt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	// zh should contain +08:00
	result := FormatMailTime(&dt, "", "zh", time.Now())
	if !containsTimezone(result, "+08:00") {
		t.Errorf("FormatMailTime(zh) = %q, expected to contain +08:00", result)
	}

	// en should contain +00:00
	result = FormatMailTime(&dt, "", "en", time.Now())
	if !containsTimezone(result, "+00:00") {
		t.Errorf("FormatMailTime(en) = %q, expected to contain +00:00", result)
	}
}

func containsTimezone(s, tz string) bool {
	return len(s) >= len(tz) && s[len(s)-len(tz):] == tz
}
