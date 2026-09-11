package config_test

import (
	"os"
	"testing"

	"kirimwa/backend/config"
)

func TestEnvBool(t *testing.T) {
	testKey := "TEST_ENV_BOOL_VAR"
	defer os.Unsetenv(testKey)

	cases := []struct {
		envVal   string
		unset    bool
		defVal   bool
		expected bool
	}{
		{unset: true, defVal: false, expected: false},
		{unset: true, defVal: true, expected: true},
		{envVal: "1", defVal: false, expected: true},
		{envVal: "true", defVal: false, expected: true},
		{envVal: "TRUE", defVal: false, expected: true},
		{envVal: "True", defVal: false, expected: true},
		{envVal: " yes ", defVal: false, expected: true},
		{envVal: "on", defVal: false, expected: true},
		{envVal: "0", defVal: true, expected: false},
		{envVal: "false", defVal: true, expected: false},
		{envVal: "FALSE", defVal: true, expected: false},
		{envVal: "False", defVal: true, expected: false},
		{envVal: " no ", defVal: true, expected: false},
		{envVal: "off", defVal: true, expected: false},
		{envVal: "unrecognized", defVal: false, expected: false},
		{envVal: "unrecognized", defVal: true, expected: true},
	}

	for _, tc := range cases {
		if tc.unset {
			os.Unsetenv(testKey)
		} else {
			os.Setenv(testKey, tc.envVal)
		}
		got := config.EnvBool(testKey, tc.defVal)
		if got != tc.expected {
			t.Errorf("EnvBool(%q, %v) with env=%q: expected %v, got %v", testKey, tc.defVal, tc.envVal, tc.expected, got)
		}
	}
}

func TestAutoMigrateEnabledDefaultFalse(t *testing.T) {
	orig := os.Getenv("AUTO_MIGRATE")
	defer func() {
		if orig != "" {
			os.Setenv("AUTO_MIGRATE", orig)
		} else {
			os.Unsetenv("AUTO_MIGRATE")
		}
	}()

	os.Unsetenv("AUTO_MIGRATE")
	if config.AutoMigrateEnabled() {
		t.Fatal("AUTO_MIGRATE default harus false jika env unset")
	}

	os.Setenv("AUTO_MIGRATE", "false")
	if config.AutoMigrateEnabled() {
		t.Fatal("AUTO_MIGRATE=false harus menghasilkan false")
	}

	os.Setenv("AUTO_MIGRATE", "0")
	if config.AutoMigrateEnabled() {
		t.Fatal("AUTO_MIGRATE=0 harus menghasilkan false")
	}

	os.Setenv("AUTO_MIGRATE", "true")
	if !config.AutoMigrateEnabled() {
		t.Fatal("AUTO_MIGRATE=true harus menghasilkan true")
	}

	os.Setenv("AUTO_MIGRATE", "1")
	if !config.AutoMigrateEnabled() {
		t.Fatal("AUTO_MIGRATE=1 harus menghasilkan true")
	}
}
