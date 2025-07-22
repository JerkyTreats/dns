package coredns

import (
	"fmt"
	"regexp"
	"strings"
)

// unicodeReplacements maps common accented characters to their ASCII equivalents
var unicodeReplacements = map[string]string{
	"à": "a", "á": "a", "â": "a", "ã": "a", "ä": "a", "å": "a", "æ": "ae",
	"ç": "c",
	"è": "e", "é": "e", "ê": "e", "ë": "e",
	"ì": "i", "í": "i", "î": "i", "ï": "i",
	"ñ": "n",
	"ò": "o", "ó": "o", "ô": "o", "õ": "o", "ö": "o", "ø": "o",
	"ù": "u", "ú": "u", "û": "u", "ü": "u",
	"ý": "y", "ÿ": "y",
	// Add more as needed
}

// sanitizeDNSName sanitizes a name to be DNS-compliant.
// DNS names must:
// - Be lowercase
// - Contain only letters (a-z), numbers (0-9), and hyphens (-)
// - Not start or end with hyphens
// - Be 63 characters or less per label
// - Not have consecutive hyphens
func sanitizeDNSName(name string) string {
	if name == "" {
		return ""
	}

	// Step 1: Convert to lowercase
	result := strings.ToLower(name)

	// Step 2: Replace underscores and dots with hyphens
	result = strings.ReplaceAll(result, "_", "-")
	result = strings.ReplaceAll(result, ".", "-")

	// Step 3: Replace common unicode characters with ASCII equivalents
	for unicode, ascii := range unicodeReplacements {
		result = strings.ReplaceAll(result, unicode, ascii)
	}

	// Step 4: Remove invalid characters (keep only a-z, 0-9, hyphen)
	// This also handles unicode characters by removing them
	validChars := regexp.MustCompile(`[^a-z0-9-]`)
	result = validChars.ReplaceAllString(result, "")

	// Step 5: Remove consecutive hyphens
	consecutiveHyphens := regexp.MustCompile(`-+`)
	result = consecutiveHyphens.ReplaceAllString(result, "-")

	// Step 6: Remove leading and trailing hyphens
	result = strings.Trim(result, "-")

	// Step 7: Enforce 63 character limit for DNS labels
	if len(result) > 63 {
		result = result[:63]
		// Remove any trailing hyphen that might have been created by truncation
		result = strings.TrimSuffix(result, "-")
	}

	return result
}

// handleDNSNameCollision generates an alternative DNS name when a collision occurs.
// It appends a numeric suffix to make the name unique.
func handleDNSNameCollision(originalName string, existingNames map[string]bool) string {
	baseName := sanitizeDNSName(originalName)
	if baseName == "" {
		return ""
	}

	// If no collision, return the base name
	if !existingNames[baseName] {
		return baseName
	}

	// Find the first available name with numeric suffix
	for i := 2; i <= 999; i++ {
		candidate := fmt.Sprintf("%s-%d", baseName, i)

		// Ensure the candidate doesn't exceed DNS length limits
		if len(candidate) > 63 {
			// Try with shorter base name
			maxBaseLen := 63 - len(fmt.Sprintf("-%d", i))
			if maxBaseLen < 1 {
				continue
			}
			candidate = fmt.Sprintf("%s-%d", baseName[:maxBaseLen], i)
			candidate = strings.TrimSuffix(candidate, "-") // Clean up any trailing hyphen
		}

		if !existingNames[candidate] {
			return candidate
		}
	}

	// If we can't find a unique name, return empty string
	return ""
}
