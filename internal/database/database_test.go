package database

import "testing"

func TestDetect(t *testing.T) {
	tests := []struct {
		name     string
		contexts []string
		want     Driver
		wantErr  bool
	}{
		{
			name:     "mysql among the default contexts",
			contexts: []string{"caddy", "mysql", "development", "caddy-development", "redis", "mailpit"},
			want:     DriverMySQL,
		},
		{
			name:     "mysql replication context",
			contexts: []string{"caddy", "mysql", "mysql-replication"},
			want:     DriverMySQL,
		},
		{
			name:     "postgres",
			contexts: []string{"caddy", "postgres"},
			want:     DriverPostgres,
		},
		{
			name:     "mssql",
			contexts: []string{"mssql"},
			want:     DriverMSSQL,
		},
		{
			name:     "sqlite",
			contexts: []string{"sqlite"},
			want:     DriverSQLite,
		},
		{
			name:     "first database context wins",
			contexts: []string{"postgres", "mysql"},
			want:     DriverPostgres,
		},
		{
			name:     "no database context",
			contexts: []string{"caddy", "redis"},
			wantErr:  true,
		},
		{
			name:     "empty",
			contexts: nil,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Detect(tt.contexts)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("Detect(%v) = %q, want an error", tt.contexts, got)
				}

				return
			}

			if err != nil {
				t.Fatalf("Detect(%v) returned error: %v", tt.contexts, err)
			}

			if got != tt.want {
				t.Errorf("Detect(%v) = %q, want %q", tt.contexts, got, tt.want)
			}
		})
	}
}

func TestInfoURL(t *testing.T) {
	tests := []struct {
		name string
		info Info
		want string
	}{
		{
			name: "mysql",
			info: Info{
				Driver:   DriverMySQL,
				Host:     "mysql.main.orb.local",
				Port:     3306,
				User:     "xf",
				Password: "password",
				Name:     "xf",
			},
			want: "mysql://xf:password@mysql.main.orb.local:3306/xf",
		},
		{
			name: "postgres uses the postgresql scheme",
			info: Info{
				Driver:   DriverPostgres,
				Host:     "postgres.main.orb.local",
				Port:     5432,
				User:     "xf",
				Password: "password",
				Name:     "xf",
			},
			want: "postgresql://xf:password@postgres.main.orb.local:5432/xf",
		},
		{
			name: "credentials are percent-encoded",
			info: Info{
				Driver:   DriverMySQL,
				Host:     "mysql.main.orb.local",
				Port:     3306,
				User:     "xf user",
				Password: "p@ss:w/rd",
				Name:     "xf",
			},
			want: "mysql://xf%20user:p%40ss%3Aw%2Frd@mysql.main.orb.local:3306/xf",
		},
		{
			name: "sqlite is a file path",
			info: Info{
				Driver:   DriverSQLite,
				FilePath: "/var/www/html/internal_data/xf.sqlite",
			},
			want: "/var/www/html/internal_data/xf.sqlite",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.info.URL(); got != tt.want {
				t.Errorf("URL() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestOrbStackHost(t *testing.T) {
	if got := OrbStackHost("mysql", "main"); got != "mysql.main.orb.local" {
		t.Errorf("OrbStackHost(mysql, main) = %q, want %q", got, "mysql.main.orb.local")
	}
}

func TestDefaultPort(t *testing.T) {
	tests := map[Driver]int{
		DriverMySQL:     3306,
		DriverPostgres:  5432,
		DriverMSSQL:     1433,
		DriverSQLite:    0,
		Driver("other"): 0,
	}

	for driver, want := range tests {
		if got := driver.DefaultPort(); got != want {
			t.Errorf("%q.DefaultPort() = %d, want %d", driver, got, want)
		}
	}
}
