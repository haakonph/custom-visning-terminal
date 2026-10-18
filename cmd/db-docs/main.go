// db-docs generates documentation using the application's SQLite migration runner.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/haakonph/custom-visning-terminal/internal/storage"
)

func main() {
	out := flag.String("out", "docs/database", "output directory")
	source := flag.String("migrations-root", "", "root containing migrations/ (default: embedded production migrations)")
	empty := flag.Bool("empty", false, "generate an empty baseline for revisions before SQLite existed")
	check := flag.Bool("check", false, "fail if checked-in documentation differs; do not write")
	flag.Parse()
	if err := run(*out, *source, *empty, *check); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(out, source string, empty, check bool) error {
	if empty && source != "" {
		return fmt.Errorf("--empty and --migrations-root are mutually exclusive")
	}
	var markdown, schema string
	var err error
	if empty {
		markdown, schema = "# Database schema\n\nSQLite did not exist at this revision.\n", "-- No SQLite schema at this revision.\n"
	} else if source != "" {
		markdown, schema, err = storage.SchemaDocumentation(os.DirFS(source))
	} else {
		markdown, schema, err = storage.SchemaDocumentation(nil)
	}
	if err != nil {
		return err
	}
	for _, doc := range []struct{ name, content string }{{"schema.md", markdown}, {"schema.sql", schema}} {
		path := filepath.Join(out, doc.name)
		if check {
			current, err := os.ReadFile(path)
			if err != nil || string(current) != doc.content {
				return fmt.Errorf("%s is missing or outdated; run npm run db:docs", path)
			}
		} else {
			if err = os.MkdirAll(out, 0755); err != nil {
				return err
			}
			if err = os.WriteFile(path, []byte(doc.content), 0644); err != nil {
				return err
			}
		}
	}
	return nil
}
