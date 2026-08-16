package scanner

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// PackageInfo contains information about a Go package.
type PackageInfo struct {
	Name      string
	Path      string
	Files     []string
	Functions []*FuncInfo
	Types     []*TypeInfo
	Imports   []string
}

// FuncInfo contains information about a function.
type FuncInfo struct {
	Name       string
	Receiver   string
	IsMethod   bool
	IsExported bool
	Line       int
}

// TypeInfo contains information about a type.
type TypeInfo struct {
	Name       string
	Kind       string // struct, interface, type alias
	IsExported bool
	Line       int
}

// ScanResult contains the result of scanning a directory.
type ScanResult struct {
	Packages    []*PackageInfo
	TotalFiles  int
	TotalLines  int
	TotalFuncs  int
	TotalTypes  int
}

// ScanDirectory scans a directory for Go files and returns package information.
func ScanDirectory(dir string) (*ScanResult, error) {
	result := &ScanResult{
		Packages: make([]*PackageInfo, 0),
	}

	packages := make(map[string]*PackageInfo)

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip hidden directories and vendor
		if info.IsDir() {
			name := info.Name()
			if strings.HasPrefix(name, ".") || name == "vendor" || name == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}

		// Only process Go files
		if !strings.HasSuffix(path, ".go") {
			return nil
		}

		result.TotalFiles++

		// Parse the file
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if err != nil {
			// Skip files that can't be parsed
			return nil
		}

		// Get or create package info
		pkgName := file.Name.Name
		pkgPath := filepath.Dir(path)
		
		key := pkgPath + ":" + pkgName
		pkg, exists := packages[key]
		if !exists {
			pkg = &PackageInfo{
				Name:      pkgName,
				Path:      pkgPath,
				Files:     make([]string, 0),
				Functions: make([]*FuncInfo, 0),
				Types:     make([]*TypeInfo, 0),
				Imports:   make([]string, 0),
			}
			packages[key] = pkg
		}

		pkg.Files = append(pkg.Files, path)

		// Count lines
		result.TotalLines += fset.File(file.Pos()).LineCount()

		// Extract imports
		importSet := make(map[string]bool)
		for _, imp := range file.Imports {
			importPath := strings.Trim(imp.Path.Value, "\"")
			if !importSet[importPath] {
				importSet[importPath] = true
				pkg.Imports = append(pkg.Imports, importPath)
			}
		}

		// Extract functions and types
		ast.Inspect(file, func(n ast.Node) bool {
			switch node := n.(type) {
			case *ast.FuncDecl:
				funcInfo := &FuncInfo{
					Name:       node.Name.Name,
					IsExported: node.Name.IsExported(),
					Line:       fset.Position(node.Pos()).Line,
				}

				if node.Recv != nil && len(node.Recv.List) > 0 {
					funcInfo.IsMethod = true
					recvType := node.Recv.List[0].Type
					if starExpr, ok := recvType.(*ast.StarExpr); ok {
						if ident, ok := starExpr.X.(*ast.Ident); ok {
							funcInfo.Receiver = ident.Name
						}
					} else if ident, ok := recvType.(*ast.Ident); ok {
						funcInfo.Receiver = ident.Name
					}
				}

				pkg.Functions = append(pkg.Functions, funcInfo)
				result.TotalFuncs++

			case *ast.TypeSpec:
				typeInfo := &TypeInfo{
					Name:       node.Name.Name,
					IsExported: node.Name.IsExported(),
					Line:       fset.Position(node.Pos()).Line,
				}

				switch node.Type.(type) {
				case *ast.StructType:
					typeInfo.Kind = "struct"
				case *ast.InterfaceType:
					typeInfo.Kind = "interface"
				default:
					typeInfo.Kind = "type"
				}

				pkg.Types = append(pkg.Types, typeInfo)
				result.TotalTypes++
			}

			return true
		})

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to walk directory: %w", err)
	}

	// Convert map to slice
	for _, pkg := range packages {
		result.Packages = append(result.Packages, pkg)
	}

	// Sort packages by path
	sort.Slice(result.Packages, func(i, j int) bool {
		return result.Packages[i].Path < result.Packages[j].Path
	})

	return result, nil
}

// GetContextSummary generates a summary of the codebase context.
func GetContextSummary(dir string) (string, error) {
	result, err := ScanDirectory(dir)
	if err != nil {
		return "", err
	}

	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("📊 Codebase Summary\n"))
	sb.WriteString(fmt.Sprintf("==================\n\n"))
	sb.WriteString(fmt.Sprintf("📁 Total Files:  %d\n", result.TotalFiles))
	sb.WriteString(fmt.Sprintf("📝 Total Lines:  %d\n", result.TotalLines))
	sb.WriteString(fmt.Sprintf("🔧 Total Functions: %d\n", result.TotalFuncs))
	sb.WriteString(fmt.Sprintf("🏷️  Total Types: %d\n\n", result.TotalTypes))

	if len(result.Packages) == 0 {
		sb.WriteString("No Go packages found.\n")
		return sb.String(), nil
	}

	sb.WriteString(fmt.Sprintf("📦 Packages (%d):\n\n", len(result.Packages)))

	for _, pkg := range result.Packages {
		sb.WriteString(fmt.Sprintf("  📁 %s\n", pkg.Name))
		sb.WriteString(fmt.Sprintf("     Path: %s\n", pkg.Path))
		sb.WriteString(fmt.Sprintf("     Files: %d\n", len(pkg.Files)))
		
		// Show exported functions
		exportedFuncs := 0
		for _, fn := range pkg.Functions {
			if fn.IsExported {
				exportedFuncs++
			}
		}
		sb.WriteString(fmt.Sprintf("     Functions: %d (%d exported)\n", len(pkg.Functions), exportedFuncs))
		
		// Show exported types
		exportedTypes := 0
		for _, t := range pkg.Types {
			if t.IsExported {
				exportedTypes++
			}
		}
		sb.WriteString(fmt.Sprintf("     Types: %d (%d exported)\n", len(pkg.Types), exportedTypes))
		
		// Show key imports (limit to 5)
		if len(pkg.Imports) > 0 {
			sb.WriteString("     Imports: ")
			limit := 5
			if len(pkg.Imports) < limit {
				limit = len(pkg.Imports)
			}
			for i, imp := range pkg.Imports[:limit] {
				if i > 0 {
					sb.WriteString(", ")
				}
				sb.WriteString(imp)
			}
			if len(pkg.Imports) > limit {
				sb.WriteString(fmt.Sprintf(" ... and %d more", len(pkg.Imports)-limit))
			}
			sb.WriteString("\n")
		}
		
		sb.WriteString("\n")
	}

	return sb.String(), nil
}

// FindGoFiles finds all Go files in a directory.
func FindGoFiles(dir string) ([]string, error) {
	var files []string

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() && strings.HasSuffix(path, ".go") {
			files = append(files, path)
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to walk directory: %w", err)
	}

	return files, nil
}

// FindFunction finds a specific function in the codebase.
func FindFunction(dir, funcName string) ([]*FuncInfo, error) {
	result, err := ScanDirectory(dir)
	if err != nil {
		return nil, err
	}

	var matches []*FuncInfo
	for _, pkg := range result.Packages {
		for _, fn := range pkg.Functions {
			if fn.Name == funcName {
				matches = append(matches, fn)
			}
		}
	}

	return matches, nil
}

// FindType finds a specific type in the codebase.
func FindType(dir, typeName string) ([]*TypeInfo, error) {
	result, err := ScanDirectory(dir)
	if err != nil {
		return nil, err
	}

	var matches []*TypeInfo
	for _, pkg := range result.Packages {
		for _, t := range pkg.Types {
			if t.Name == typeName {
				matches = append(matches, t)
			}
		}
	}

	return matches, nil
}
