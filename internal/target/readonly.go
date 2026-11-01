package target

import (
	"errors"
	"fmt"
	"strings"
)

// CheckReadOnly reports an error unless query is a single SELECT or SHOW
// statement. Every query modhak sends to a target passes through it, so
// "never execute DDL" (ADR 0007) is enforced at runtime and not only by
// convention. Leading whitespace and comments are skipped.
func CheckReadOnly(query string) error {
	q := stripLeadingComments(query)
	word := strings.ToUpper(firstWord(q))
	if word != "SELECT" && word != "SHOW" {
		return fmt.Errorf("refusing to run %q statement: only SELECT and SHOW are allowed", word)
	}
	if hasExtraStatement(q) {
		return errors.New("refusing to run multiple statements")
	}
	return nil
}

func stripLeadingComments(q string) string {
	for {
		q = strings.TrimLeft(q, " \t\r\n")
		switch {
		case strings.HasPrefix(q, "/*"):
			end := strings.Index(q, "*/")
			if end < 0 {
				return ""
			}
			q = q[end+2:]
		case strings.HasPrefix(q, "--"), strings.HasPrefix(q, "#"):
			nl := strings.IndexByte(q, '\n')
			if nl < 0 {
				return ""
			}
			q = q[nl+1:]
		default:
			return q
		}
	}
}

func firstWord(q string) string {
	end := strings.IndexFunc(q, func(r rune) bool { return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r == '_') })
	if end < 0 {
		return q
	}
	return q[:end]
}

// hasExtraStatement reports a semicolon outside quotes that is followed by
// anything but whitespace.
func hasExtraStatement(q string) bool {
	var quote byte
	for i := 0; i < len(q); i++ {
		c := q[i]
		switch {
		case quote != 0:
			if c == '\\' {
				i++
			} else if c == quote {
				quote = 0
			}
		case c == '\'' || c == '"' || c == '`':
			quote = c
		case c == ';':
			return strings.TrimSpace(q[i+1:]) != ""
		}
	}
	return false
}
