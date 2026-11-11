package compose

import (
	"strings"
	"testing"
)

// A managed database's own start settings are named in a note rather than dropped quietly, and a password in its command never is.
func TestManagedDatabaseSettingsAreNoted(t *testing.T) {
	p := BuildPlan("shop", mustParse(t, `services:
  db:
    image: postgres:16
    command: ["postgres", "-c", "max_connections=300"]
    environment: {POSTGRES_PASSWORD: pw}
    volumes: ["./postgresql.conf:/etc/postgresql/postgresql.conf", "./pgdata:/var/lib/postgresql/data"]
  sql:
    image: mysql:8
    environment: {MYSQL_USER: app, MYSQL_PASSWORD: apppw, MYSQL_ROOT_PASSWORD: rootpw, MYSQL_DATABASE: app}
  cache:
    image: redis:7
    command: redis-server --requirepass s3cret
  cache2:
    image: redis:7
    command: redis-server --requirepass=eq5ecret --appendonly yes
`), noFiles, func(string) PathKind { return PathFile })
	notes := strings.Join(p.Notes, "\n")
	for _, want := range []string{
		"db: is a managed database, so its command's -c max_connections=300 isn't used",
		"db: is a managed database, so the file ./postgresql.conf mounted at /etc/postgresql/postgresql.conf isn't used",
		"db: starts empty, so the data in ./pgdata isn't brought over",
		"sql: its root password becomes the same as app's",
	} {
		if !strings.Contains(notes, want) {
			t.Errorf("no note containing %q in:\n%s", want, notes)
		}
	}
	if strings.Contains(notes, "cache: is a managed database, so its command") || strings.Contains(notes, "s3cret") || strings.Contains(notes, "eq5ecret") {
		t.Errorf("redis's own command was noted:\n%s", notes)
	}
}

func TestRequirepassEqualsFormIsTheDatabasePassword(t *testing.T) {
	p := BuildPlan("s", mustParse(t, `services:
  cache:
    image: redis:7
    command: redis-server --requirepass=eq5ecret
`), noFiles, func(string) PathKind { return PathFile })
	if db := item(t, p, "cache"); db.DBPassword != "eq5ecret" {
		t.Errorf("password = %q", db.DBPassword)
	}
	if got := ignoredFlags([]string{"redis-server", "--requirepass=x", "--requirepass", "y"}, ""); len(got) != 0 {
		t.Errorf("ignored = %v", got)
	}
}

func mustParse(t *testing.T, src string) *File {
	t.Helper()
	f, err := Parse([]byte(src), noEnv)
	if err != nil {
		t.Fatal(err)
	}
	return f
}
