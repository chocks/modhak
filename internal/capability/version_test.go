package capability

import "testing"

func TestParseServerInfo(t *testing.T) {
	tests := []struct {
		name, version, comment string
		wantFlavor             Flavor
		want                   Version
		wantErr                bool
	}{
		{"mariadb 11", "11.4.2-MariaDB-ubu2404", "mariadb.org binary distribution", FlavorMariaDB, Version{11, 4, 2}, false},
		{"mariadb 10 with 5.5.5 prefix", "5.5.5-10.11.6-MariaDB-1:10.11.6+maria~ubu2204", "mariadb.org binary distribution", FlavorMariaDB, Version{10, 11, 6}, false},
		{"mariadb debian log suffix", "10.6.12-MariaDB-log", "", FlavorMariaDB, Version{10, 6, 12}, false},
		{"mariadb flavor only in comment", "10.5.9", "MariaDB Server", FlavorMariaDB, Version{10, 5, 9}, false},
		{"mysql 8.4", "8.4.0", "MySQL Community Server - GPL", FlavorMySQL, Version{8, 4, 0}, false},
		{"mysql 5.7 log", "5.7.44-log", "MySQL Community Server (GPL)", FlavorMySQL, Version{5, 7, 44}, false},
		{"mysql ubuntu suffix", "8.0.35-0ubuntu0.22.04.1", "(Ubuntu)", FlavorMySQL, Version{8, 0, 35}, false},
		{"percona fork", "8.0.34-26", "Percona Server (GPL), Release 26, Revision 0fe62c9e", FlavorMySQL, Version{8, 0, 34}, false},
		{"rds style custom suffix", "8.0.36-rds.20240101", "Source distribution", FlavorMySQL, Version{8, 0, 36}, false},
		{"5.5.5 prefix without mariadb is not stripped", "5.5.5-foo", "", FlavorMySQL, Version{5, 5, 5}, false},
		{"two-part version", "9.1", "", FlavorMySQL, Version{9, 1, 0}, false},
		{"surrounding space", "  8.0.1 ", "", FlavorMySQL, Version{8, 0, 1}, false},
		{"empty", "", "", FlavorUnknown, Version{}, true},
		{"garbage", "banana", "", FlavorUnknown, Version{}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseServerInfo(tt.version, tt.comment)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil {
				return
			}
			if got.Flavor != tt.wantFlavor || got.Version != tt.want {
				t.Fatalf("got %s %s, want %s %s", got.Flavor, got.Version, tt.wantFlavor, tt.want)
			}
			if got.Raw != tt.version || got.Comment != tt.comment {
				t.Fatalf("raw/comment not preserved: %+v", got)
			}
		})
	}
}

func TestAtLeast(t *testing.T) {
	v := Version{10, 6, 0}
	tests := []struct {
		maj, min, pat int
		want          bool
	}{
		{10, 6, 0, true}, {10, 5, 9, true}, {10, 6, 1, false}, {10, 7, 0, false}, {9, 99, 99, true}, {11, 0, 0, false},
	}
	for _, tt := range tests {
		if got := v.AtLeast(tt.maj, tt.min, tt.pat); got != tt.want {
			t.Errorf("%s.AtLeast(%d,%d,%d) = %v", v, tt.maj, tt.min, tt.pat, got)
		}
	}
}
