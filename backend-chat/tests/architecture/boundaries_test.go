package architecture_test

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const modulePath = "github.com/Felipe-candido/Barber-chat"

// This checks production imports, not runtime tenant isolation or SQL atomicity.
func TestBusinessLayerImportBoundaries(t *testing.T) {
	root := filepath.Join("..", "..", "internal", "modules")
	filesChecked := 0
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		parts := strings.Split(filepath.ToSlash(relative), "/")
		if len(parts) < 3 || (parts[1] != "domain" && parts[1] != "application") {
			return nil
		}

		positions := token.NewFileSet()
		file, err := parser.ParseFile(positions, path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		filesChecked++
		for _, imported := range file.Imports {
			importPath, err := strconv.Unquote(imported.Path.Value)
			if err != nil {
				return err
			}
			if reason := importViolation(parts[0], parts[1], importPath); reason != "" {
				t.Errorf("%s: import %q violates %s boundary: %s", positions.Position(imported.Pos()), importPath, parts[1], reason)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("inspect business-layer imports: %v", err)
	}
	if filesChecked == 0 {
		t.Fatal("no production business-layer files were inspected")
	}
}

func TestImportBoundaryRules(t *testing.T) {
	tests := []struct {
		name       string
		layer      string
		importPath string
		allowed    bool
	}{
		{"standard library", "domain", "time", true},
		{"uuid value type", "domain", "github.com/google/uuid", true},
		{"own domain", "application", modulePath + "/internal/modules/booking/domain", true},
		{"own domain subpackage", "domain", modulePath + "/internal/modules/booking/domain/calendar", true},
		{"own application", "application", modulePath + "/internal/modules/booking/application/rules", true},
		{"domain importing application", "domain", modulePath + "/internal/modules/booking/application", false},
		{"own infra", "application", modulePath + "/internal/modules/booking/infra/postgres", false},
		{"another module domain", "application", modulePath + "/internal/modules/catalog/domain", false},
		{"another module application", "application", modulePath + "/internal/modules/customers/application", false},
		{"generated queries", "application", modulePath + "/internal/database/sqlc", false},
		{"platform", "application", modulePath + "/internal/platform/postgres", false},
		{"http context", "application", modulePath + "/internal/httpapi/requestctx", false},
		{"domain prefix lookalike", "application", modulePath + "/internal/modules/booking/domain_extra", false},
		{"http framework", "application", "github.com/go-chi/chi/v5", false},
		{"http transport", "domain", "net/http", false},
		{"sql driver", "application", "github.com/jackc/pgx/v5/pgxpool", false},
		{"standard sql", "domain", "database/sql", false},
		{"broker", "application", "github.com/rabbitmq/amqp091-go", false},
		{"jwt adapter dependency", "application", "github.com/golang-jwt/jwt/v5", false},
		{"supabase sdk", "application", "github.com/supabase-community/supabase-go", false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			allowed := importViolation("booking", test.layer, test.importPath) == ""
			if allowed != test.allowed {
				t.Errorf("import %q allowed=%t, want %t", test.importPath, allowed, test.allowed)
			}
		})
	}
}

func importViolation(module, layer, importPath string) string {
	if strings.HasPrefix(importPath, modulePath+"/") {
		ownModule := modulePath + "/internal/modules/" + module + "/"
		if isWithin(importPath, ownModule+"domain") || (layer == "application" && isWithin(importPath, ownModule+"application")) {
			return ""
		}
		return "depend on consumer-owned ports, not infrastructure or another module's packages"
	}

	for _, forbidden := range []string{
		"net/http",
		"database/sql",
		"github.com/go-chi/chi",
		"github.com/jackc/pgx",
		"github.com/rabbitmq/amqp091-go",
		"github.com/go-jose/go-jose",
		"github.com/golang-jwt/jwt",
		"github.com/supabase-community",
		"github.com/supabase",
	} {
		if isWithin(importPath, forbidden) {
			return "transport, provider and persistence dependencies belong in adapters"
		}
	}
	return ""
}

func isWithin(importPath, root string) bool {
	return importPath == root || strings.HasPrefix(importPath, root+"/")
}
