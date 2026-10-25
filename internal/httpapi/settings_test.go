package httpapi

import (
	"encoding/json"
	"testing"
)

func TestLocaleSettingKeyIsEditable(t *testing.T) {
	expected := "i18n" + "." + "default" + "_" + "locale"
	if localeSettingKey != expected {
		t.Fatalf("locale key mismatch: %q", localeSettingKey)
	}
	if !editableSettings[localeSettingKey] {
		t.Fatalf("locale key is not editable: %q", localeSettingKey)
	}
	if err := validateSettingValue(localeSettingKey, json.RawMessage(`"ko"`)); err != nil {
		t.Fatalf("locale value rejected: %v", err)
	}
	if err := validateSettingValue(localeSettingKey, json.RawMessage(`"fr"`)); err == nil {
		t.Fatal("unsupported locale should be rejected")
	}
}
