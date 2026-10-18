package web_test

import (
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// skippedFieldsFile lists, one per line and sorted, each field of tablo's
// view types that tablo gives and the adapters of a view do not read, as
// "<view> <Type>.<Field>".
const skippedFieldsFile = "testdata/skipped-fields.txt"

// viewPkg is the package of tablo's view types.
const viewPkg = "github.com/nbyoung/tablo/view"

// viewRoots names the type of tablo's data for each view the adapters draw.
var viewRoots = map[string]string{
	"gates": "Gates", "task": "Task", "authority": "Delegation", "assignment": "Assignment",
	"queue": "Queue", "blockage": "Blockage", "tableau": "Tableau", "context": "Tableau",
	"history": "History", "audit": "Audit",
}

// TestSkippedFields holds what tablo gives and the adapters do not draw, as
// tabloio's T17 holds the keys it skips (tablo 493e and 886d decision 16 b).
// It type-checks this package's source and reads, for each view, the fields
// of tablo's view types that the view's adapter selects: in the function
// that adapters[view] names, in envOf, which every view's Env passes
// through, and in every function of the package those reach. A value of a
// view type passed whole to json.Marshal, the brief that tabloio's package
// composes, counts as read in full; tabloio's T17 lists what it skips. From
// the view's type of data the test walks every field the adapters read into
// its own type, and lists each field they do not read. A field the file does
// not list fails, and so does a listed field that is read or no longer
// exists, and a file out of order. The type check imports from source, so
// the test skips without the go command, which locates the modules.
func TestSkippedFields(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go is not on the path: the type check locates tablo's module with it")
	}
	t.Setenv("GOFLAGS", "-mod=readonly")
	t.Setenv("GOPROXY", "off")
	t.Setenv("GOCACHE", t.TempDir())
	fset := token.NewFileSet()
	dir, err := filepath.Abs(".")
	if err != nil {
		t.Fatal(err)
	}
	names, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	var files []*ast.File
	for _, name := range names {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, f)
	}
	info := &types.Info{
		Selections: map[*ast.SelectorExpr]*types.Selection{},
		Uses:       map[*ast.Ident]types.Object{},
		Defs:       map[*ast.Ident]types.Object{},
		Types:      map[ast.Expr]types.TypeAndValue{},
	}
	conf := types.Config{Importer: importer.ForCompiler(fset, "source", nil)}
	pkg, err := conf.Check("github.com/nbyoung/tableaud/internal/web", fset, files, info)
	if err != nil {
		t.Fatalf("type check: %v", err)
	}

	// The functions of the package, and the roots of each view.
	bodies := map[*types.Func]*ast.BlockStmt{}
	roots := map[string]*ast.FuncLit{}
	for _, f := range files {
		for _, d := range f.Decls {
			fd, ok := d.(*ast.FuncDecl)
			if !ok || fd.Body == nil {
				continue
			}
			if fn, ok := info.Defs[fd.Name].(*types.Func); ok {
				bodies[fn] = fd.Body
			}
			if fd.Name.Name != "init" {
				continue
			}
			ast.Inspect(fd.Body, func(n ast.Node) bool {
				as, ok := n.(*ast.AssignStmt)
				if !ok || len(as.Lhs) != 1 || len(as.Rhs) != 1 {
					return true
				}
				ix, ok := as.Lhs[0].(*ast.IndexExpr)
				lit, isLit := as.Rhs[0].(*ast.FuncLit)
				if !ok || !isLit {
					return true
				}
				if id, ok := ix.X.(*ast.Ident); ok && id.Name == "adapters" {
					if key, ok := ix.Index.(*ast.BasicLit); ok {
						roots[strings.Trim(key.Value, `"`)] = lit
					}
				}
				return true
			})
		}
	}
	envOf, _ := pkg.Scope().Lookup("envOf").(*types.Func)
	if envOf == nil || bodies[envOf] == nil {
		t.Fatal("the package declares no envOf")
	}
	vp := viewPackage(pkg)
	if vp == nil {
		t.Fatalf("the package imports no %s", viewPkg)
	}

	skipped := map[string]bool{}
	counts := map[string]int{}
	for view, root := range viewRoots {
		lit := roots[view]
		if lit == nil {
			t.Errorf("adapters names no adapter of %s", view)
			continue
		}
		read, whole := reads(info, bodies, []ast.Node{lit.Body, bodies[envOf]})
		named, _ := vp.Scope().Lookup(root).Type().(*types.Named)
		if named == nil {
			t.Errorf("%s declares no %s", viewPkg, root)
			continue
		}
		seen := map[*types.Named]bool{}
		var walk func(n *types.Named)
		walk = func(n *types.Named) {
			if seen[n] {
				return
			}
			seen[n] = true
			st, ok := n.Underlying().(*types.Struct)
			if !ok {
				return
			}
			for i := range st.NumFields() {
				f := st.Field(i)
				key := n.Obj().Name() + "." + f.Name()
				if !read[key] && !whole[n] {
					skipped[view+" "+key] = true
					counts[view]++
					continue
				}
				for _, inner := range namedIn(f.Type()) {
					if whole[n] {
						whole[inner] = true
					}
					walk(inner)
				}
			}
		}
		walk(named)
	}
	checkSkippedFields(t, skipped)
	var per []string
	for view := range viewRoots {
		per = append(per, view+" "+strconv.Itoa(counts[view]))
	}
	slices.Sort(per)
	t.Logf("the adapters skip %d fields of tablo's view types: %s", len(skipped), strings.Join(per, ", "))
}

// viewPackage returns tablo's package view as the package imports it.
func viewPackage(pkg *types.Package) *types.Package {
	for _, p := range pkg.Imports() {
		if p.Path() == viewPkg {
			return p
		}
	}
	return nil
}

// reads returns the fields of tablo's view types, as "Type.Field", that the
// code under roots selects, with every function of the package it reaches,
// and the view types whose values it passes whole to json.Marshal. A field
// promoted through an embedded field reads the embedded field too.
func reads(info *types.Info, bodies map[*types.Func]*ast.BlockStmt, roots []ast.Node) (map[string]bool, map[*types.Named]bool) {
	read, whole := map[string]bool{}, map[*types.Named]bool{}
	done := map[ast.Node]bool{}
	queue := slices.Clone(roots)
	for len(queue) > 0 {
		body := queue[0]
		queue = queue[1:]
		if done[body] {
			continue
		}
		done[body] = true
		ast.Inspect(body, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.Ident:
				if fn, ok := info.Uses[x].(*types.Func); ok {
					if b := bodies[fn.Origin()]; b != nil {
						queue = append(queue, b)
					}
				}
			case *ast.SelectorExpr:
				sel := info.Selections[x]
				if sel == nil || sel.Kind() != types.FieldVal {
					return true
				}
				t := sel.Recv()
				for _, i := range sel.Index() {
					n := namedOf(t)
					if n == nil {
						break
					}
					st, ok := n.Underlying().(*types.Struct)
					if !ok {
						break
					}
					f := st.Field(i)
					if n.Obj().Pkg() != nil && n.Obj().Pkg().Path() == viewPkg {
						read[n.Obj().Name()+"."+f.Name()] = true
					}
					t = f.Type()
				}
			case *ast.CallExpr:
				fs, ok := x.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				if fn, ok := info.Uses[fs.Sel].(*types.Func); ok && fn.Pkg() != nil && fn.Pkg().Path() == "encoding/json" && fn.Name() == "Marshal" {
					for _, a := range x.Args {
						if n := namedOf(info.TypeOf(a)); n != nil && n.Obj().Pkg() != nil && n.Obj().Pkg().Path() == viewPkg {
							whole[n] = true
						}
					}
				}
			}
			return true
		})
	}
	return read, whole
}

// namedOf returns the named type of t, through one pointer.
func namedOf(t types.Type) *types.Named {
	if p, ok := t.(*types.Pointer); ok {
		t = p.Elem()
	}
	n, _ := t.(*types.Named)
	return n
}

// namedIn returns the named struct types of tablo's view package that a
// field's type holds: itself, or the element of a pointer, a slice, an array
// or a map.
func namedIn(t types.Type) []*types.Named {
	for {
		switch x := t.(type) {
		case *types.Pointer:
			t = x.Elem()
		case *types.Slice:
			t = x.Elem()
		case *types.Array:
			t = x.Elem()
		case *types.Map:
			return append(namedIn(x.Key()), namedIn(x.Elem())...)
		case *types.Named:
			if x.Obj().Pkg() == nil || x.Obj().Pkg().Path() != viewPkg {
				return nil
			}
			if _, ok := x.Underlying().(*types.Struct); !ok {
				return nil
			}
			return []*types.Named{x}
		default:
			return nil
		}
	}
}

// checkSkippedFields compares the fields the adapters skip with
// skippedFieldsFile: a skipped field the file does not list fails, and so
// does a listed field that no view skips, or a file that is not sorted.
func checkSkippedFields(t *testing.T, skipped map[string]bool) {
	t.Helper()
	raw, err := os.ReadFile(skippedFieldsFile)
	if err != nil {
		t.Fatal(err)
	}
	listed := strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n")
	if !slices.IsSorted(listed) {
		t.Errorf("%s is not sorted", skippedFieldsFile)
	}
	inFile := map[string]bool{}
	for _, k := range listed {
		inFile[k] = true
		if !skipped[k] {
			t.Errorf("%s lists %q, which the adapters read or tablo no longer gives", skippedFieldsFile, k)
		}
	}
	var unlisted []string
	for k := range skipped {
		if !inFile[k] {
			unlisted = append(unlisted, k)
		}
	}
	slices.Sort(unlisted)
	for _, k := range unlisted {
		t.Errorf("the adapters skip %q, which %s does not list", k, skippedFieldsFile)
	}
}
