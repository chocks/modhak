package advisor

import "strings"

// quote returns a backtick-quoted identifier, doubling embedded backticks.
func quote(name string) string { return "`" + strings.ReplaceAll(name, "`", "``") + "`" }

func qualified(schema, table string) string { return quote(schema) + "." + quote(table) }

func dropIndexSQL(schema, table, index string) string {
	return "ALTER TABLE " + qualified(schema, table) + " DROP INDEX " + quote(index)
}

func findingID(kind string, t Target) string {
	parts := []string{kind, t.Name, t.Schema, t.Table}
	if t.Index != "" {
		parts = append(parts, t.Index)
	}
	if t.Column != "" {
		parts = append(parts, t.Column)
	}
	return strings.Join(parts, ":")
}
