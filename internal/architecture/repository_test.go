package architecture

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
)

type goFile struct {
	abs       string
	rel       string
	dir       string
	syntax    *ast.File
	imports   []string
	lines     int
	generated bool
}

type repository struct {
	root       string
	module     string
	fset       *token.FileSet
	production []goFile
	byDir      map[string][]goFile
	graph      map[string][]string
	tests      func() ([]goFile, error)
}

var sharedRepository = sync.OnceValues(readRepository)

// loadRepository returns the repository parsed once for the whole test run.
func loadRepository(t *testing.T) *repository {
	t.Helper()

	repo, err := sharedRepository()
	if err != nil {
		t.Fatalf("load repository: %v", err)
	}
	return repo
}

func readRepository() (*repository, error) {
	root, err := findModuleRoot()
	if err != nil {
		return nil, err
	}
	module, err := readModulePath(filepath.Join(root, "go.mod"))
	if err != nil {
		return nil, err
	}
	productionPaths, testPaths, err := listGoFiles(root)
	if err != nil {
		return nil, err
	}
	repo := &repository{root: root, module: module, fset: token.NewFileSet()}
	if repo.production, err = parseFiles(repo.fset, productionPaths); err != nil {
		return nil, err
	}
	repo.byDir = groupByDir(repo.production)
	repo.graph = importGraph(repo.production, module)
	repo.tests = sync.OnceValues(func() ([]goFile, error) { return parseFiles(repo.fset, testPaths) })
	return repo, nil
}

func findModuleRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("find module root: %w", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("find module root: no go.mod above the test directory")
		}
		dir = parent
	}
}

// readModulePath extracts the module directive from a go.mod file.
func readModulePath(goModPath string) (string, error) {
	body, err := os.ReadFile(goModPath)
	if err != nil {
		return "", fmt.Errorf("read go.mod: %w", err)
	}
	for line := range strings.SplitSeq(string(body), "\n") {
		if module, ok := strings.CutPrefix(strings.TrimSpace(line), "module "); ok {
			return strings.TrimSpace(module), nil
		}
	}
	return "", errors.New("go.mod does not declare a module path")
}

type goPath struct {
	abs string
	rel string
}

// listGoFiles walks the repository and splits its Go files into production and
// test files, skipping hidden directories, testdata, vendor, tmp and bin.
func listGoFiles(root string) (production, tests []goPath, err error) {
	err = filepath.WalkDir(root, func(abs string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return skipDirectory(root, abs, entry.Name())
		}
		if !strings.HasSuffix(abs, ".go") {
			return nil
		}
		rel, err := filepath.Rel(root, abs)
		if err != nil {
			return err
		}
		file := goPath{abs: abs, rel: filepath.ToSlash(rel)}
		if strings.HasSuffix(abs, "_test.go") {
			tests = append(tests, file)
		} else {
			production = append(production, file)
		}
		return nil
	})
	if err != nil {
		return nil, nil, fmt.Errorf("walk repository: %w", err)
	}
	return production, tests, nil
}

func skipDirectory(root, abs, name string) error {
	if abs != root && (strings.HasPrefix(name, ".") || name == "testdata" || name == "vendor" || name == "tmp" || name == "bin") {
		return filepath.SkipDir
	}
	return nil
}

func parseFiles(fset *token.FileSet, paths []goPath) ([]goFile, error) {
	files := make([]goFile, len(paths))
	failures := make([]error, len(paths))
	slots := make(chan struct{}, runtime.GOMAXPROCS(0))
	var group sync.WaitGroup
	for i, source := range paths {
		group.Go(func() {
			slots <- struct{}{}
			defer func() { <-slots }()
			files[i], failures[i] = parseFile(fset, source)
		})
	}
	group.Wait()
	return files, errors.Join(failures...)
}

func parseFile(fset *token.FileSet, source goPath) (goFile, error) {
	content, err := os.ReadFile(source.abs)
	if err != nil {
		return goFile{}, fmt.Errorf("read %s: %w", source.rel, err)
	}
	syntax, err := parser.ParseFile(fset, source.abs, content, parser.ParseComments)
	if err != nil {
		return goFile{}, fmt.Errorf("parse %s: %w", source.rel, err)
	}
	imports, err := importPaths(syntax)
	if err != nil {
		return goFile{}, fmt.Errorf("%s: %w", source.rel, err)
	}
	lines, generated := countLines(content)
	return goFile{abs: source.abs, rel: source.rel, dir: path.Dir(source.rel), syntax: syntax, imports: imports, lines: lines, generated: generated}, nil
}

func importPaths(syntax *ast.File) ([]string, error) {
	paths := make([]string, 0, len(syntax.Imports))
	for _, spec := range syntax.Imports {
		value, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			return nil, fmt.Errorf("unquote import %s: %w", spec.Path.Value, err)
		}
		paths = append(paths, value)
	}
	return paths, nil
}

// countLines returns the line count of a source file and whether it carries
// the standard "Code generated ... DO NOT EDIT." marker.
func countLines(content []byte) (int, bool) {
	lines := 0
	generated := false
	scanner := bufio.NewScanner(bytes.NewReader(content))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		lines++
		text := scanner.Text()
		if strings.HasPrefix(text, "// Code generated ") && strings.HasSuffix(text, " DO NOT EDIT.") {
			generated = true
		}
	}
	return lines, generated
}

func groupByDir(files []goFile) map[string][]goFile {
	byDir := map[string][]goFile{}
	for _, file := range files {
		byDir[file.dir] = append(byDir[file.dir], file)
	}
	return byDir
}

// importGraph maps each package directory, relative to the module root, to the
// sorted module-internal packages its production files import.
func importGraph(files []goFile, module string) map[string][]string {
	graph := map[string][]string{}
	for _, file := range files {
		if file.dir == "." || strings.HasPrefix(file.dir, "examples/") {
			continue
		}
		imports := graph[file.dir]
		for _, imported := range file.imports {
			if rest, ok := strings.CutPrefix(imported, module+"/"); ok && !slices.Contains(imports, rest) {
				imports = append(imports, rest)
			}
		}
		slices.Sort(imports)
		graph[file.dir] = imports
	}
	return graph
}

// filesIn lists the production files of the package directory dir. A gate
// that finds none fails, so a renamed package cannot silently disable it.
func (r *repository) filesIn(t *testing.T, dir string) []goFile {
	t.Helper()

	files := r.byDir[dir]
	if len(files) == 0 {
		t.Fatalf("no production files in %s; update the gate that names it", dir)
	}
	return files
}

// filesUnder lists the production files of dir and every directory below it.
func (r *repository) filesUnder(dir string) []goFile {
	var files []goFile
	for _, file := range r.production {
		if file.dir == dir || strings.HasPrefix(file.dir, dir+"/") {
			files = append(files, file)
		}
	}
	return files
}

// layerFiles lists the production files of every internal/<module>/internal/<layer>.
func (r *repository) layerFiles(layer string) []goFile {
	var files []goFile
	for _, file := range r.production {
		if path.Base(file.dir) == layer && path.Base(path.Dir(file.dir)) == "internal" {
			files = append(files, file)
		}
	}
	return files
}

// allFiles lists every Go file of the repository, test files included.
func (r *repository) allFiles(t *testing.T) []goFile {
	t.Helper()

	tests, err := r.tests()
	if err != nil {
		t.Fatalf("parse test files: %v", err)
	}
	return append(slices.Clone(r.production), tests...)
}

// position renders file:line for a node of any parsed file.
func (r *repository) position(node ast.Node) string {
	position := r.fset.Position(node.Pos())
	rel, err := filepath.Rel(r.root, position.Filename)
	if err != nil {
		return position.String()
	}
	return fmt.Sprintf("%s:%d", filepath.ToSlash(rel), position.Line)
}

func TestNoTestsAtRepositoryRoot(t *testing.T) {
	t.Parallel()

	entries, err := os.ReadDir(loadRepository(t).root)
	if err != nil {
		t.Fatalf("read repository root: %v", err)
	}
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), "_test.go") {
			t.Errorf("%s is a test at the repository root; tests live with the code they prove, repository-wide gates in internal/architecture", entry.Name())
		}
	}
}
