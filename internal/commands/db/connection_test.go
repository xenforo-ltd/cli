package db

import "testing"

func TestDetect(t *testing.T) {
	tests := []struct {
		name     string
		contexts []string
		want     driver
		wantErr  bool
	}{
		{
			name:     "mysql among the default contexts",
			contexts: []string{"caddy", "mysql", "development", "caddy-development", "redis", "mailpit"},
			want:     driverMySQL,
		},
		{
			name:     "mysql replication context",
			contexts: []string{"caddy", "mysql-replication"},
			want:     driverMySQL,
		},
		{
			name:     "postgres",
			contexts: []string{"caddy", "postgres"},
			want:     driverPostgres,
		},
		{
			name:     "mssql",
			contexts: []string{"mssql"},
			want:     driverMSSQL,
		},
		{
			name:     "sqlite",
			contexts: []string{"sqlite"},
			want:     driverSQLite,
		},
		{
			name:     "first database context wins",
			contexts: []string{"postgres", "mysql"},
			want:     driverPostgres,
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
			got, err := detect(tt.contexts)
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
		info connection
		want string
	}{
		{
			name: "mysql",
			info: connection{
				Driver:   driverMySQL,
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
			info: connection{
				Driver:   driverPostgres,
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
			info: connection{
				Driver:   driverMySQL,
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
			info: connection{
				Driver:   driverSQLite,
				FilePath: "/var/www/html/internal_data/xf.sqlite",
			},
			want: "/var/www/html/internal_data/xf.sqlite",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.info.urlString(); got != tt.want {
				t.Errorf("URL() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestOrbStackHost(t *testing.T) {
	if got := orbStackHost("mysql", "main"); got != "mysql.main.orb.local" {
		t.Errorf("OrbStackHost(mysql, main) = %q, want %q", got, "mysql.main.orb.local")
	}
}

func TestDefaultPort(t *testing.T) {
	tests := map[driver]int{
		driverMySQL:     3306,
		driverPostgres:  5432,
		driverMSSQL:     1433,
		driverSQLite:    0,
		driver("other"): 0,
	}

	for driver, want := range tests {
		if got := driver.defaultPort(); got != want {
			t.Errorf("%q.DefaultPort() = %d, want %d", driver, got, want)
		}
	}
}
