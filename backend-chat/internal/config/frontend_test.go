package config

import "testing"

func TestFrontendOrigin(t *testing.T) {
	for _, origin := range []string{"", "http://127.0.0.1:3000", "http://localhost:3000", "http://[::1]:3000", "https://evil.example", "http://192.0.2.1:3000", "http://localhost:3000/", "http://localhost:3000?x=1", "http://user@localhost:3000", "http://localhost:0", "http://localhost:99999", "null", "*"} {
		t.Run(origin, func(t *testing.T) {
			_, err := load(func(key string) string {
				return map[string]string{"DATABASE_URL": "postgres://localhost/test", "DEV_FRONTEND_ORIGIN": origin}[key]
			})
			allowed := origin == "" || origin == "http://127.0.0.1:3000" || origin == "http://localhost:3000" || origin == "http://[::1]:3000"
			if (err == nil) != allowed {
				t.Fatalf("allowed=%v error=%v", allowed, err)
			}
		})
	}
	_, err := load(func(key string) string {
		return map[string]string{"DATABASE_URL": "postgres://localhost/test", "DEV_FRONTEND_ORIGIN": "http://localhost:3000", "HTTP_ADDR": "0.0.0.0:8080"}[key]
	})
	if err == nil {
		t.Fatal("browser access must require loopback listener")
	}
}
