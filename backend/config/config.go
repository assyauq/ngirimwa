package config

import (
	"log"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

func loadEnv() {
	// Cari .env di direktori aktif atau parent directory (jika dijalankan dari subfolder backend/)
	paths := []string{".env", "../.env", "../../.env"}
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			_ = godotenv.Load(p)
			return
		}
	}
	_ = godotenv.Load()
}

// init memuat .env SEKALI, SEBELUM variabel package-level lain (mis. jwtSecret) diinisialisasi.
// Karena package config diimpor paling awal, ini cukup — tak perlu godotenv.Load() lagi di main().
func init() {
	loadEnv()
}

// Load disediakan untuk command/seed lama yang masih memanggil config.Load().
// init() tetap menjadi mekanisme utama agar env tersedia sebelum package-level var lain dibuat.
func Load() {
	loadEnv()
}

func Env(key, defaultVal string) string {
	v := os.Getenv(key)
	if v == "" {
		return defaultVal
	}
	return v
}

func EnvRequired(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("ERROR: %s harus diset di .env", key)
	}
	return v
}

func EnvInt(key string, defaultVal int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return defaultVal
}

func EnvBool(key string, defaultVal bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return defaultVal
	}
	switch v {
	case "1", "true", "TRUE", "yes", "on":
		return true
	case "0", "false", "FALSE", "no", "off":
		return false
	default:
		return defaultVal
	}
}
