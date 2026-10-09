// Package config loads settings from the environment, optionally seeded from
// a .env file. Real environment variables always win over the file.
package config

import (
	"bufio"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Port            string   // listen port, e.g. "8080"
	FrontendOrigins []string // browser origins allowed by CORS (FRONTEND_URL, comma-separated)
}

// Load reads path (if it exists) into the process environment, then validates.
func Load(path string) (Config, error) {
	if err := loadDotEnv(path); err != nil {
		return Config{}, err
	}
	c := Config{
		Port: os.Getenv("PORT"),
	}
	if c.Port == "" {
		return Config{}, errors.New("PORT is required (set it in .env)")
	}
	if n, err := strconv.Atoi(c.Port); err != nil || n < 1 || n > 65535 {
		return Config{}, fmt.Errorf("PORT %q is not a valid port", c.Port)
	}
	for _, v := range strings.Split(os.Getenv("FRONTEND_URL"), ",") {
		if strings.TrimSpace(v) == "" {
			continue
		}
		o, err := parseOrigin("FRONTEND_URL", v)
		if err != nil {
			return Config{}, err
		}
		c.FrontendOrigins = append(c.FrontendOrigins, o)
	}
	return c, nil
}

// parseOrigin returns v as a bare http(s) origin (no path, no trailing slash).
func parseOrigin(key, v string) (string, error) {
	v = strings.TrimSpace(v)
	u, err := url.Parse(v)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" ||
		(u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" || u.User != nil {
		return "", fmt.Errorf("%s %q must be an origin like https://example.com (scheme + host, no path)", key, v)
	}
	return u.Scheme + "://" + u.Host, nil
}

func loadDotEnv(path string) error {
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil // fine: everything may come from the real environment
	}
	if err != nil {
		return err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		k = strings.TrimSpace(strings.TrimPrefix(k, "export "))
		if !ok || k == "" {
			return fmt.Errorf("%s:%d: expected KEY=VALUE", path, n)
		}
		v = strings.TrimSpace(v)
		if len(v) >= 2 && (v[0] == '"' || v[0] == '\'') && v[len(v)-1] == v[0] {
			v = v[1 : len(v)-1]
		}
		if _, set := os.LookupEnv(k); !set {
			os.Setenv(k, v)
		}
	}
	return sc.Err()
}
