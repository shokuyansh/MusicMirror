package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Port                                                   string
	Production                                             bool
	DatabaseURL, SupabaseURL, SupabaseKey, Bucket, DataDir string
	Origins                                                []string
	MaxUpload, MaxStorage                                  int64
	TrustProxy                                             bool
}

func Load() (Config, error) { return From(os.Getenv) }
func From(get func(string) string) (Config, error) {
	if env := get("APP_ENV"); env != "" && env != "development" && env != "production" {
		return Config{}, errors.New("APP_ENV must be development or production")
	}
	value := func(key, fallback string) string {
		if v := get(key); v != "" {
			return v
		}
		return fallback
	}
	c := Config{Port: value("PORT", "3001"), Production: get("APP_ENV") == "production", DatabaseURL: get("DATABASE_URL"), SupabaseURL: strings.TrimRight(get("SUPABASE_URL"), "/"), SupabaseKey: get("SUPABASE_SECRET_KEY"), Bucket: value("SUPABASE_BUCKET", "music"), DataDir: value("DATA_DIR", "data"), TrustProxy: get("TRUST_PROXY") == "true"}
	for _, origin := range strings.Split(value("ALLOWED_ORIGINS", "http://localhost:5173"), ",") {
		origin = strings.TrimSpace(origin)
		u, err := url.Parse(origin)
		if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || u.User != nil {
			return c, errors.New("ALLOWED_ORIGINS must contain exact HTTP(S) origins without paths")
		}
		if c.Production && u.Scheme != "https" {
			return c, errors.New("production origins must use HTTPS")
		}
		c.Origins = append(c.Origins, origin)
	}
	var err error
	c.MaxUpload, err = strconv.ParseInt(value("MAX_UPLOAD_MB", "20"), 10, 64)
	if err != nil || c.MaxUpload < 1 || c.MaxUpload > 50 {
		return c, errors.New("MAX_UPLOAD_MB must be between 1 and 50")
	}
	c.MaxStorage, err = strconv.ParseInt(value("MAX_STORAGE_MB", "500"), 10, 64)
	if err != nil || c.MaxStorage < c.MaxUpload || c.MaxStorage > 900 {
		return c, errors.New("MAX_STORAGE_MB must be between MAX_UPLOAD_MB and 900")
	}
	c.MaxUpload *= 1 << 20
	c.MaxStorage *= 1 << 20
	port, err := strconv.Atoi(c.Port)
	if err != nil || port < 1 || port > 65535 {
		return c, errors.New("PORT must be a valid TCP port")
	}
	if c.SupabaseURL != "" {
		u, err := url.Parse(c.SupabaseURL)
		if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" {
			return c, errors.New("SUPABASE_URL must be the HTTPS project origin")
		}
	}
	if strings.ContainsAny(c.Bucket, "/\\ .") || c.Bucket == "" {
		return c, errors.New("invalid SUPABASE_BUCKET")
	}
	if c.Production || c.DatabaseURL != "" || c.SupabaseURL != "" || c.SupabaseKey != "" {
		if c.DatabaseURL == "" || c.SupabaseURL == "" || c.SupabaseKey == "" {
			return c, errors.New("DATABASE_URL, SUPABASE_URL and SUPABASE_SECRET_KEY must all be set")
		}
		u, err := url.Parse(c.DatabaseURL)
		if err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") || u.Host == "" {
			return c, errors.New("DATABASE_URL must be a PostgreSQL connection URI")
		}
		if c.Production && u.Query().Get("sslmode") != "require" && u.Query().Get("sslmode") != "verify-full" {
			return c, fmt.Errorf("production DATABASE_URL requires sslmode=require or verify-full")
		}
		query := u.Query()
		query.Set("connect_timeout", "10")
		u.RawQuery = query.Encode()
		c.DatabaseURL = u.String()
	}
	return c, nil
}
