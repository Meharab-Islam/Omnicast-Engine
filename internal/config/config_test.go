package config

import (
	"os"
	"testing"
)

func TestGenerateRandomKey(t *testing.T) {
	key1 := GenerateRandomKey(16)
	key2 := GenerateRandomKey(16)

	if len(key1) != 32 {
		t.Fatalf("Expected 32 hex chars for 16 bytes, got %d (%s)", len(key1), key1)
	}
	if key1 == key2 {
		t.Fatalf("Expected random keys to be unique, got duplicate: %s", key1)
	}
}

func TestLoadOrGenerateConfig_WithYAML(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "lms_config_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	oldWd, _ := os.Getwd()
	_ = os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	cfg := LoadOrGenerateConfig()
	if cfg.APIKey == "" || cfg.APISecret == "" || cfg.JWTSecret == "" || cfg.TurnSecret == "" {
		t.Fatalf("Expected auto-generated credentials, got empty fields: %+v", cfg)
	}

	// Verify persistence of config.yaml and .env
	if _, err := os.Stat("config/config.yaml"); os.IsNotExist(err) {
		t.Fatalf("Expected config/config.yaml to be created automatically")
	}
	if _, err := os.Stat(".env"); os.IsNotExist(err) {
		t.Fatalf("Expected .env file to be created and persisted in working directory")
	}

	// Verify default UNLIMITED rules (0 = Unlimited)
	if cfg.YAML.RoomManagement.MaxViewersPerRoom != 0 {
		t.Fatalf("Expected default MaxViewersPerRoom = 0 (Unlimited), got %d", cfg.YAML.RoomManagement.MaxViewersPerRoom)
	}
	if cfg.YAML.CoHosting.MaxActiveSeats != 0 {
		t.Fatalf("Expected default MaxActiveSeats = 0 (Unlimited), got %d", cfg.YAML.CoHosting.MaxActiveSeats)
	}
	if !cfg.YAML.WebRTC.SimulcastEnabled {
		t.Fatalf("Expected SimulcastEnabled = true")
	}

	// Verify GetAppConfig returns the loaded configuration
	appCfg := GetAppConfig()
	if appCfg.YAML.RoomManagement.MaxViewersPerRoom != 0 {
		t.Fatalf("Expected GetAppConfig() to match global loaded config")
	}
}
