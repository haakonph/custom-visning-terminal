package storage

import (
	"database/sql"
	"strings"
	"testing"
	"testing/fstest"
)

func TestSchemaDocumentation(t *testing.T) {
	md, ddl, err := SchemaDocumentation(nil)
	if err != nil {
		t.Fatal(err)
	}
	again, againSQL, err := SchemaDocumentation(nil)
	if err != nil || md != again || ddl != againSQL {
		t.Fatal("generation is not deterministic", err)
	}
	for _, want := range []string{"erDiagram", "TEXT pull_request_id PK,FK", "accounts |o..o{ github_repositories"} {
		if !strings.Contains(md, want) {
			t.Errorf("missing %q", want)
		}
	}
	for _, want := range []string{"CREATE TABLE schema_migrations", "CREATE VIEW overview_items", "CREATE INDEX github_pr_updated", "CREATE TABLE documents"} {
		if !strings.Contains(ddl, want) {
			t.Errorf("missing %q", want)
		}
	}
	if strings.Contains(ddl, "INSERT INTO") || strings.Contains(ddl, "sqlite_autoindex") {
		t.Fatal("dump contains data or internal indexes")
	}
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err = db.Exec(ddl); err != nil {
		t.Fatal("dump cannot be restored", err)
	}
	restoredMD, restoredSQL, err := describeSchema(db)
	if err != nil || restoredMD != md || restoredSQL != ddl {
		t.Fatal("schema roundtrip differs", err)
	}
}

func TestSchemaDocumentationRunsAlterAndIncludesAllObjects(t *testing.T) {
	source := fstest.MapFS{
		"migrations/000_versions.sql": {Data: []byte("CREATE TABLE schema_migrations(version INTEGER PRIMARY KEY);")},
		"migrations/001_initial.sql":  {Data: []byte(`CREATE TABLE parent(a TEXT, b TEXT, PRIMARY KEY(a,b)); CREATE TABLE child(a TEXT, b TEXT, FOREIGN KEY(a,b) REFERENCES parent(a,b));`)},
		"migrations/002_change.sql":   {Data: []byte(`ALTER TABLE child ADD COLUMN extra TEXT; CREATE INDEX child_extra ON child(extra); CREATE VIEW summary AS SELECT extra FROM child; CREATE TRIGGER child_insert AFTER INSERT ON child BEGIN UPDATE child SET extra = 'changed'; END; CREATE TABLE "odd-name"("field name" TEXT);`)},
	}
	md, ddl, err := SchemaDocumentation(source)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(md, "parent |o..o{ child") != 1 {
		t.Fatal("composite FK must produce one relationship")
	}
	for _, want := range []string{"TEXT extra", "TEXT a FK", "encoded_6f64642d6e616d65", "encoded_6669656c64206e616d65"} {
		if !strings.Contains(md, want) {
			t.Errorf("missing %q", want)
		}
	}
	for _, want := range []string{"CREATE TRIGGER", "CREATE INDEX", "CREATE VIEW", "extra TEXT"} {
		if !strings.Contains(ddl, want) {
			t.Errorf("missing %q", want)
		}
	}
	source["migrations/002_change.sql"] = &fstest.MapFile{Data: []byte("INVALID SQL;")}
	if _, _, err := SchemaDocumentation(source); err == nil {
		t.Fatal("invalid migration succeeded")
	}
	if _, _, err := SchemaDocumentation(fstest.MapFS{}); err == nil {
		t.Fatal("missing migrations accepted as empty baseline")
	}
}
