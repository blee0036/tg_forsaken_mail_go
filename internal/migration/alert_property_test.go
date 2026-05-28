package migration

import (
	"sort"
	"strings"
	"testing"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"

	"go-version-rewrite/internal/config"
)

// Feature: bot-migration, Property 3: Alert 语言回退解析
// Validates: Requirements 2.3
func TestProperty_AlertLanguageFallbackResolution(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100

	properties := gopter.NewProperties(parameters)

	// Generator for non-empty map[string]string with non-empty keys and values
	nonEmptyAlertMapGen := gen.MapOf(
		gen.Weighted([]gen.WeightedGen{
			{Weight: 3, Gen: gen.AlphaString()},
			{Weight: 2, Gen: gen.OneConstOf("en", "zh", "fr", "de", "ja", "ko", "es")},
		}),
		gen.Weighted([]gen.WeightedGen{
			{Weight: 5, Gen: gen.AlphaString()},
			{Weight: 2, Gen: gen.OneConstOf("Switch to new bot", "请切换到新 Bot", "Wechseln Sie")},
		}),
	).SuchThat(func(m map[string]string) bool {
		// Ensure map is non-empty and all keys/values are non-blank
		if len(m) == 0 {
			return false
		}
		for k, v := range m {
			if strings.TrimSpace(k) == "" || strings.TrimSpace(v) == "" {
				return false
			}
		}
		return true
	})

	// Generator for language codes
	langGen := gen.Weighted([]gen.WeightedGen{
		{Weight: 3, Gen: gen.AlphaString()},
		{Weight: 2, Gen: gen.OneConstOf("en", "zh", "fr", "de", "ja", "ko", "es")},
		{Weight: 2, Gen: gen.OneConstOf("zh-Hans", "zh-Hant", "zh_CN", "en-US", "en-GB", "fr-FR")},
	})

	// Property 3a: For any non-empty map and any language, result is non-empty
	properties.Property("ResolveAlertMessage returns non-empty string for non-empty map", prop.ForAll(
		func(msgs map[string]string, lang string) bool {
			cfg := &config.Config{ChangeBotAlertMsg: msgs}
			a := NewAlertSender(cfg)

			result := a.ResolveAlertMessage(lang)
			if result == "" {
				t.Logf("Got empty result for lang=%q, map=%v", lang, msgs)
				return false
			}
			return true
		},
		nonEmptyAlertMapGen,
		langGen,
	))

	// Property 3b: The returned string must be one of the values in the map
	properties.Property("ResolveAlertMessage returns a value that exists in the map", prop.ForAll(
		func(msgs map[string]string, lang string) bool {
			cfg := &config.Config{ChangeBotAlertMsg: msgs}
			a := NewAlertSender(cfg)

			result := a.ResolveAlertMessage(lang)

			// Check that result is one of the values in the map
			for _, v := range msgs {
				if result == v {
					return true
				}
			}
			t.Logf("Result %q not found in map values, lang=%q, map=%v", result, lang, msgs)
			return false
		},
		nonEmptyAlertMapGen,
		langGen,
	))

	// Property 3c: If exact language is in the map, it must return that exact value
	properties.Property("ResolveAlertMessage returns exact match when language key exists", prop.ForAll(
		func(msgs map[string]string, lang string) bool {
			// Only test when lang is actually a key in the map
			expectedVal, exists := msgs[lang]
			if !exists {
				return true // skip - not testing this case here
			}

			cfg := &config.Config{ChangeBotAlertMsg: msgs}
			a := NewAlertSender(cfg)

			result := a.ResolveAlertMessage(lang)
			if result != expectedVal {
				t.Logf("Expected exact match %q for lang=%q, got %q", expectedVal, lang, result)
				return false
			}
			return true
		},
		nonEmptyAlertMapGen,
		langGen,
	))

	// Property 3d: Prefix normalization - if lang has separator and prefix is in map, returns prefix value
	properties.Property("ResolveAlertMessage uses prefix normalization when exact match not found", prop.ForAll(
		func(msgs map[string]string, prefix string, suffix string) bool {
			// Skip if prefix is empty or already in map as full lang
			if prefix == "" {
				return true
			}

			// Construct a language code with separator
			lang := prefix + "-" + suffix

			// Skip if the full lang is already in the map (exact match takes priority)
			if _, exists := msgs[lang]; exists {
				return true
			}

			// Only test when prefix IS in the map
			expectedVal, prefixExists := msgs[prefix]
			if !prefixExists {
				return true // skip - not testing this case here
			}

			cfg := &config.Config{ChangeBotAlertMsg: msgs}
			a := NewAlertSender(cfg)

			result := a.ResolveAlertMessage(lang)
			if result != expectedVal {
				t.Logf("Expected prefix match %q for lang=%q (prefix=%q), got %q", expectedVal, lang, prefix, result)
				return false
			}
			return true
		},
		nonEmptyAlertMapGen,
		gen.Weighted([]gen.WeightedGen{
			{Weight: 3, Gen: gen.OneConstOf("en", "zh", "fr", "de", "ja")},
			{Weight: 2, Gen: gen.AlphaString()},
		}),
		gen.AlphaString(), // suffix after separator
	))

	// Property 3e: Fallback chain - when no exact/prefix match, falls back to en > zh > first alphabetically
	properties.Property("ResolveAlertMessage follows fallback chain: en > zh > first alphabetically", prop.ForAll(
		func(msgs map[string]string, lang string) bool {
			// Skip if exact match or prefix match would apply
			if _, exists := msgs[lang]; exists {
				return true
			}
			if idx := strings.IndexAny(lang, "-_"); idx > 0 {
				prefix := lang[:idx]
				if _, exists := msgs[prefix]; exists {
					return true
				}
			}

			cfg := &config.Config{ChangeBotAlertMsg: msgs}
			a := NewAlertSender(cfg)

			result := a.ResolveAlertMessage(lang)

			// Determine expected fallback
			if enVal, ok := msgs["en"]; ok {
				if result != enVal {
					t.Logf("Expected 'en' fallback %q, got %q (lang=%q)", enVal, result, lang)
					return false
				}
				return true
			}
			if zhVal, ok := msgs["zh"]; ok {
				if result != zhVal {
					t.Logf("Expected 'zh' fallback %q, got %q (lang=%q)", zhVal, result, lang)
					return false
				}
				return true
			}

			// Should be first key alphabetically
			keys := make([]string, 0, len(msgs))
			for k := range msgs {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			expectedVal := msgs[keys[0]]
			if result != expectedVal {
				t.Logf("Expected first-alphabetically fallback %q (key=%q), got %q (lang=%q)", expectedVal, keys[0], result, lang)
				return false
			}
			return true
		},
		nonEmptyAlertMapGen,
		gen.Weighted([]gen.WeightedGen{
			// Use language codes unlikely to be in the random map
			{Weight: 5, Gen: gen.OneConstOf("xx", "yy", "zz", "qq", "ww")},
			{Weight: 2, Gen: gen.AlphaString()},
		}),
	))

	properties.TestingRun(t)
}
