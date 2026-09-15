package v1

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

var (
	_ func(Config) (*Client, error) = NewClient
	_ func(error, Kind) bool        = IsKind
	_                               = Config{
		Endpoint:   string(""),
		Token:      string(""),
		UserAgent:  string(""),
		HTTPClient: HTTPClient(nil),
	}
	_ = Response{StatusCode: int(0), RequestID: string("")}
	_ = Error{
		Kind:            Kind(""),
		Message:         string(""),
		StatusCode:      int(0),
		RequestID:       string(""),
		StructuredFault: bool(false),
		Err:             error(nil),
	}
)

func TestPublicAPISurface(t *testing.T) {
	t.Parallel()

	expectedDeclarations := map[string][]string{
		".": {
			"Client",
			"Config",
			"Error",
			"HTTPClient",
			"IsKind",
			"Kind",
			"KindCanceled",
			"KindConflict",
			"KindForbidden",
			"KindIncompleteList",
			"KindInvalidRequest",
			"KindNotFound",
			"KindOverQuota",
			"KindRateLimited",
			"KindServerError",
			"KindTimeout",
			"KindTransport",
			"KindUnexpected",
			"NewClient",
			"Response",
		},
		"image": {
			"Create",
			"CreateOpts",
			"CreateOpts.MarshalJSON",
			"Delete",
			"Get",
			"Import",
			"ImportMethodWebDownload",
			"List",
			"ListOpts",
			"OpAdd",
			"OpRemove",
			"OpReplace",
			"PatchOp",
			"StatusActive",
			"StatusDeactivated",
			"StatusDeleted",
			"StatusImporting",
			"StatusKilled",
			"StatusPendingDelete",
			"StatusQueued",
			"StatusSaving",
			"Task",
			"TaskStatusFailure",
			"TaskStatusPending",
			"TaskStatusProcessing",
			"TaskStatusSuccess",
			"TaskTypeImport",
			"Tasks",
			"Timestamp",
			"Timestamp.UnmarshalJSON",
			"Update",
			"Upload",
			"View",
			"View.UnmarshalJSON",
			"VisibilityCommunity",
			"VisibilityPrivate",
			"VisibilityPublic",
			"VisibilityShared",
		},
		"member": {
			"Create",
			"Delete",
			"Get",
			"List",
			"StatusAccepted",
			"StatusPending",
			"StatusRejected",
			"Update",
			"View",
		},
	}

	for directory, expected := range expectedDeclarations {
		t.Run(directory, func(t *testing.T) {
			t.Parallel()

			require.ElementsMatch(t, expected, exportedDeclarations(t, directory))
		})
	}

	assertExportedMethods(t, reflect.TypeOf((*HTTPClient)(nil)).Elem(), "Do")
	assertExportedMethods(t, reflect.TypeOf((*Client)(nil)))
	assertExportedMethods(t, reflect.TypeOf((*Error)(nil)), "Error", "Unwrap")

	for _, typ := range []reflect.Type{
		reflect.TypeOf(Kind("")),
		reflect.TypeOf(Response{}),
	} {
		assertExportedMethods(t, typ)
	}

	assertExportedFields(t, reflect.TypeOf(Response{}), "StatusCode", "RequestID")
	assertExportedFields(t, reflect.TypeOf(Config{}), "Endpoint", "Token", "UserAgent", "HTTPClient")
	assertExportedFields(
		t,
		reflect.TypeOf(Error{}),
		"Kind",
		"Message",
		"StatusCode",
		"RequestID",
		"StructuredFault",
		"Err",
	)
	assertExportedFields(t, reflect.TypeOf((*Client)(nil)).Elem())
}

func exportedDeclarations(t *testing.T, directory string) []string {
	t.Helper()

	entries, err := os.ReadDir(directory)
	require.NoError(t, err)

	fileSet := token.NewFileSet()
	result := make([]string, 0)

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}

		path := filepath.Join(directory, entry.Name())
		file, err := parser.ParseFile(fileSet, path, nil, 0)
		require.NoError(t, err)

		for _, declaration := range file.Decls {
			result = append(result, exportedDeclarationNames(t, declaration)...)
		}
	}

	sort.Strings(result)

	return result
}

func exportedDeclarationNames(t *testing.T, declaration ast.Decl) []string {
	t.Helper()

	switch declaration := declaration.(type) {
	case *ast.GenDecl:
		return exportedSpecificationNames(declaration.Specs)
	case *ast.FuncDecl:
		return exportedFunctionName(t, declaration)
	default:
		return nil
	}
}

func exportedSpecificationNames(specifications []ast.Spec) []string {
	result := make([]string, 0)

	for _, specification := range specifications {
		switch specification := specification.(type) {
		case *ast.TypeSpec:
			if specification.Name.IsExported() {
				result = append(result, specification.Name.Name)
			}
		case *ast.ValueSpec:
			for _, name := range specification.Names {
				if name.IsExported() {
					result = append(result, name.Name)
				}
			}
		}
	}

	return result
}

func exportedFunctionName(t *testing.T, declaration *ast.FuncDecl) []string {
	t.Helper()

	if !declaration.Name.IsExported() {
		return nil
	}

	name := declaration.Name.Name
	if declaration.Recv == nil {
		return []string{name}
	}

	receiver := receiverName(t, declaration.Recv.List[0].Type)
	if !ast.IsExported(receiver) {
		return nil
	}

	return []string{receiver + "." + name}
}

func receiverName(t *testing.T, expression ast.Expr) string {
	t.Helper()

	switch expression := expression.(type) {
	case *ast.Ident:
		return expression.Name
	case *ast.StarExpr:
		return receiverName(t, expression.X)
	default:
		t.Fatalf("unsupported receiver expression %T", expression)

		return ""
	}
}

func assertExportedMethods(t *testing.T, typ reflect.Type, expected ...string) {
	t.Helper()

	actual := make([]string, 0, typ.NumMethod())
	for index := 0; index < typ.NumMethod(); index++ {
		actual = append(actual, typ.Method(index).Name)
	}

	require.ElementsMatch(t, expected, actual, "unexpected method set of %s", typ)
}

func assertExportedFields(t *testing.T, typ reflect.Type, expected ...string) {
	t.Helper()

	actual := make([]string, 0, typ.NumField())
	for index := 0; index < typ.NumField(); index++ {
		field := typ.Field(index)
		if field.IsExported() {
			actual = append(actual, field.Name)
		}
	}

	require.ElementsMatch(t, expected, actual, "unexpected public fields of %s", typ)
}
