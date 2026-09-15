package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

type constant struct {
	typ   ast.Expr
	value ast.Expr
}

type inventory struct {
	Types     map[string]string            `json:"types"`
	Constants map[string]map[string]string `json:"constants"`
}

func main() {
	if len(os.Args) != 2 {
		fail(fmt.Errorf("usage: gateway-contract-inventory <provider source directory>"))
	}
	fset := token.NewFileSet()
	entries, err := os.ReadDir(os.Args[1])
	if err != nil {
		fail(err)
	}
	files := make([]*ast.File, 0)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, filepath.Join(os.Args[1], entry.Name()), nil, 0)
		if err != nil {
			fail(err)
		}
		if file.Name.Name == "provider" {
			files = append(files, file)
		}
	}
	if len(files) == 0 {
		fail(fmt.Errorf("provider source package not found"))
	}
	types := map[string]ast.Expr{}
	aliases := map[string]ast.Expr{}
	constants := map[string]constant{}
	for _, file := range files {
		for _, declaration := range file.Decls {
			general, ok := declaration.(*ast.GenDecl)
			if !ok {
				continue
			}
			var previousType ast.Expr
			var previousValues []ast.Expr
			for _, specification := range general.Specs {
				switch spec := specification.(type) {
				case *ast.TypeSpec:
					types[spec.Name.Name] = spec.Type
					if spec.Assign.IsValid() {
						aliases[spec.Name.Name] = spec.Type
					}
				case *ast.ValueSpec:
					if general.Tok != token.CONST {
						continue
					}
					if len(spec.Values) != 0 {
						previousType, previousValues = spec.Type, spec.Values
					}
					for index, name := range spec.Names {
						if index >= len(previousValues) {
							fail(fmt.Errorf("unsupported constant declaration %s", name))
						}
						constants[name.Name] = constant{previousType, previousValues[index]}
					}
				}
			}
		}
	}
	if types["CallOptions"] == nil {
		fail(fmt.Errorf("provider.CallOptions not found"))
	}
	printNode := func(node ast.Node) string {
		var buffer bytes.Buffer
		if err := format.Node(&buffer, fset, node); err != nil {
			fail(err)
		}
		return buffer.String()
	}
	result := inventory{Types: map[string]string{}, Constants: map[string]map[string]string{}}
	var visit func(ast.Expr)
	visit = func(expression ast.Expr) {
		switch value := expression.(type) {
		case *ast.Ident:
			definition, exists := types[value.Name]
			if !exists {
				return
			}
			if _, visited := result.Types[value.Name]; visited {
				return
			}
			result.Types[value.Name] = printNode(definition)
			visit(definition)
		case *ast.StarExpr:
			visit(value.X)
		case *ast.ArrayType:
			visit(value.Elt)
		case *ast.MapType:
			visit(value.Key)
			visit(value.Value)
		case *ast.StructType:
			for _, field := range value.Fields.List {
				visit(field.Type)
			}
		case *ast.InterfaceType:
			for _, field := range value.Methods.List {
				visit(field.Type)
			}
		case *ast.FuncType:
			for _, fields := range []*ast.FieldList{value.Params, value.Results} {
				if fields != nil {
					for _, field := range fields.List {
						visit(field.Type)
					}
				}
			}
		case *ast.ChanType:
			visit(value.Value)
		case *ast.Ellipsis:
			visit(value.Elt)
		case *ast.ParenExpr:
			visit(value.X)
		case *ast.SelectorExpr:
		default:
			fail(fmt.Errorf("unclassified contract type %T", expression))
		}
	}
	visit(ast.NewIdent("CallOptions"))
	var constantType func(constant, map[string]bool) string
	constantType = func(value constant, seen map[string]bool) string {
		if value.typ != nil {
			return printNode(value.typ)
		}
		switch expression := value.value.(type) {
		case *ast.Ident:
			if alias, exists := constants[expression.Name]; exists && !seen[expression.Name] {
				seen[expression.Name] = true
				return constantType(alias, seen)
			}
		case *ast.CallExpr:
			return printNode(expression.Fun)
		case *ast.ParenExpr:
			return constantType(constant{value: expression.X}, seen)
		case *ast.UnaryExpr:
			return constantType(constant{value: expression.X}, seen)
		case *ast.BinaryExpr:
			if typ := constantType(constant{value: expression.X}, seen); typ != "" {
				return typ
			}
			return constantType(constant{value: expression.Y}, seen)
		}
		return ""
	}
	for name, value := range constants {
		typ := constantType(value, map[string]bool{})
		seen := map[string]bool{}
		for aliases[typ] != nil && !seen[typ] {
			seen[typ] = true
			typ = printNode(aliases[typ])
		}
		if _, relevant := result.Types[typ]; !relevant {
			continue
		}
		if result.Constants[typ] == nil {
			result.Constants[typ] = map[string]string{}
		}
		result.Constants[typ][name] = printNode(value.value)
	}
	if err := json.NewEncoder(os.Stdout).Encode(result); err != nil {
		fail(err)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
