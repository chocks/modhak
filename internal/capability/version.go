package capability

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Flavor is the database family a target belongs to.
type Flavor string

const (
	FlavorUnknown Flavor = "unknown"
	FlavorMariaDB Flavor = "mariadb"
	FlavorMySQL   Flavor = "mysql"
)

// Version is a parsed server version.
type Version struct {
	Major, Minor, Patch int
}

func (v Version) String() string { return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch) }

// AtLeast reports whether v >= major.minor.patch.
func (v Version) AtLeast(major, minor, patch int) bool {
	if v.Major != major {
		return v.Major > major
	}
	if v.Minor != minor {
		return v.Minor > minor
	}
	return v.Patch >= patch
}

// ServerInfo is what VERSION() and @@version_comment say about a server.
type ServerInfo struct {
	Flavor  Flavor
	Version Version
	// Raw is the VERSION() string as reported; Comment is @@version_comment.
	Raw     string
	Comment string
}

// MariaDB versions through 10.x were reported with a "5.5.5-" prefix for
// replication-protocol compatibility. Strip it so 5.5.5-10.11.6-MariaDB
// parses as 10.11.6. The prefix is only removed when MariaDB follows.
var mariaPrefix = regexp.MustCompile(`^5\.5\.5-(\d+\.\d+)`)

var numericRE = regexp.MustCompile(`^(\d+)\.(\d+)(?:\.(\d+))?`)

// ParseServerInfo interprets VERSION() and @@version_comment (comment may be
// empty). Flavor is MariaDB if either string mentions it, otherwise MySQL;
// forks and managed services (Percona, RDS, ...) count as MySQL or MariaDB
// by the version they report. Fork-specific handling is TBD (FEATURES.md).
func ParseServerInfo(version, comment string) (ServerInfo, error) {
	info := ServerInfo{Flavor: FlavorUnknown, Raw: version, Comment: comment}
	v := strings.TrimSpace(version)
	if v == "" {
		return info, fmt.Errorf("empty version string")
	}
	isMaria := strings.Contains(strings.ToLower(v), "mariadb") ||
		strings.Contains(strings.ToLower(comment), "mariadb")
	if isMaria && mariaPrefix.MatchString(v) {
		v = v[len("5.5.5-"):]
	}
	m := numericRE.FindStringSubmatch(v)
	if m == nil {
		return info, fmt.Errorf("cannot parse version %q", version)
	}
	info.Version.Major, _ = strconv.Atoi(m[1])
	info.Version.Minor, _ = strconv.Atoi(m[2])
	if m[3] != "" {
		info.Version.Patch, _ = strconv.Atoi(m[3])
	}
	if isMaria {
		info.Flavor = FlavorMariaDB
	} else {
		info.Flavor = FlavorMySQL
	}
	return info, nil
}
