package config

import "testing"

func TestDefaultsAndUnsafeProductionConfiguration(t *testing.T) {
	c, err := From(func(string) string { return "" })
	if err != nil || c.MaxUpload != 20<<20 || c.Port != "3001" {
		t.Fatalf("%+v %v", c, err)
	}
	for _, env := range []map[string]string{
		{"APP_ENV": "prod"},
		{"APP_ENV": "production"},
		{"MAX_UPLOAD_MB": "51"},
		{"MAX_STORAGE_MB": "1"},
		{"ALLOWED_ORIGINS": "*"},
		{"ALLOWED_ORIGINS": "https://example.com/path"},
		{"PORT": "no"},
		{"SUPABASE_URL": "https://project.supabase.co"},
	} {
		if _, err := From(func(key string) string { return env[key] }); err == nil {
			t.Fatal("accepted invalid configuration", env)
		}
	}
	env := map[string]string{"APP_ENV": "production", "ALLOWED_ORIGINS": "https://music.example.com", "DATABASE_URL": "postgres://user:password@db.example.com/postgres?sslmode=require", "SUPABASE_URL": "https://project.supabase.co", "SUPABASE_SECRET_KEY": "secret"}
	if _, err := From(func(key string) string { return env[key] }); err != nil {
		t.Fatal(err)
	}
}
