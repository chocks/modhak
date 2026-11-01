package target

import "testing"

func TestCheckReadOnly(t *testing.T) {
	tests := []struct {
		name    string
		query   string
		wantErr bool
	}{
		{"select", "SELECT 1", false},
		{"lowercase select", "select * from t", false},
		{"show", "SHOW GLOBAL STATUS LIKE 'Uptime'", false},
		{"leading space and newline", "  \n\tSELECT 1", false},
		{"leading block comment", "/* hi */ SELECT 1", false},
		{"leading line comments", "-- a\n# b\nSELECT 1", false},
		{"trailing semicolon", "SELECT 1;", false},
		{"trailing semicolon and space", "SELECT 1; \n", false},
		{"semicolon inside string", "SELECT 'a;b'", false},
		{"semicolon inside backticks", "SELECT `a;b` FROM t", false},
		{"escaped quote then semicolon", `SELECT 'it\'s; fine'`, false},
		{"insert", "INSERT INTO t VALUES (1)", true},
		{"update", "UPDATE t SET a=1", true},
		{"delete", "DELETE FROM t", true},
		{"drop", "DROP INDEX i ON t", true},
		{"alter", "ALTER TABLE t DROP INDEX i", true},
		{"create", "CREATE TABLE t (a INT)", true},
		{"truncate", "TRUNCATE TABLE t", true},
		{"kill", "KILL QUERY 1", true},
		{"set", "SET GLOBAL userstat=1", true},
		{"flush", "FLUSH INDEX_STATISTICS", true},
		{"with write", "WITH x AS (SELECT 1) DELETE FROM t", true},
		{"select then drop", "SELECT 1; DROP TABLE t", true},
		{"select then drop after comment", "SELECT 1;/* x */DROP TABLE t", true},
		{"comment hides write", "/* SELECT */ DROP TABLE t", true},
		{"empty", "", true},
		{"only comment", "-- nothing", true},
		{"selectx is not select", "SELECTX 1", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := CheckReadOnly(tt.query); (err != nil) != tt.wantErr {
				t.Fatalf("CheckReadOnly(%q) = %v, wantErr %v", tt.query, err, tt.wantErr)
			}
		})
	}
}
