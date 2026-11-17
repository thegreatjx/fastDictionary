package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGoroutinesRecoverPanics(t *testing.T) {
	allowedLiterals := map[string]string{
		"audio.go": "go func() { _ = c.Wait() }()",
	}

	functionVars := map[string]string{
		"OnMouseDown":  "onGlobalMouseDown",
		"OnMouseWheel": "onGlobalMouseWheel",
		"OnMouseMove":  "onChipPointer",
		"OnKeyDown":    "onGlobalKeyDown",
	}

	externalCalls := map[string]string{
		"OpenExternal": "mygo 的 Shell.OpenExternal，只启动浏览器，不碰应用状态",
	}

	fset := token.NewFileSet()
	decls := map[string]*ast.BlockStmt{}
	files := map[string]*ast.File{}

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		files[name] = f
		for _, d := range f.Decls {
			fd, ok := d.(*ast.FuncDecl)
			if !ok || fd.Body == nil {
				continue
			}
			decls[funcKey(fd)] = fd.Body
		}
	}

	for name, f := range files {
		ast.Inspect(f, func(n ast.Node) bool {
			g, ok := n.(*ast.GoStmt)
			if !ok {
				return true
			}
			line := fset.Position(g.Pos()).Line

			if lit, ok := g.Call.Fun.(*ast.FuncLit); ok {
				if lit.Body != nil && !hasRecoverCall(lit.Body) {
					if want, ok := allowedLiterals[name]; ok && nodeText(fset, g) == want {
						return true
					}
					t.Errorf("%s:%d 启动了裸 goroutine：panic 会终止整个进程，给它加 defer recoverLog(...)",
						name, line)
				}
				return true
			}

			key := callKey(g.Call.Fun)
			if key == "" {
				t.Errorf("%s:%d 的 goroutine 目标无法识别，请在测试里登记", name, line)
				return true
			}

			if body, ok := lookup(decls, key); ok {
				if !hasRecoverCall(body) {
					t.Errorf("%s:%d 的 goroutine 目标函数 %s 没有 recoverLog：panic 会终止整个进程",
						name, line, key)
				}
				return true
			}

			if target, ok := functionVars[key]; ok {
				body, found := lookup(decls, target)
				switch {
				case !found:
					t.Errorf("%s:%d 登记的函数变量 %s 指向 %s，但找不到该函数", name, line, key, target)
				case !hasRecoverCall(body):
					t.Errorf("%s:%d 的 goroutine 经函数变量 %s 调用 %s，后者没有 recoverLog",
						name, line, key, target)
				}
				return true
			}

			if _, ok := externalCalls[key]; ok {
				return true
			}

			t.Errorf("%s:%d 的 goroutine 目标 %s 找不到函数体：跨包调用或未登记的函数变量，请在测试里登记理由",
				name, line, key)
			return true
		})
	}
}

func funcKey(fd *ast.FuncDecl) string {
	if fd.Recv == nil || len(fd.Recv.List) == 0 {
		return fd.Name.Name
	}
	t := fd.Recv.List[0].Type
	if star, ok := t.(*ast.StarExpr); ok {
		t = star.X
	}
	if id, ok := t.(*ast.Ident); ok {
		return id.Name + "." + fd.Name.Name
	}
	return fd.Name.Name
}

func lookup(decls map[string]*ast.BlockStmt, key string) (*ast.BlockStmt, bool) {
	if body, ok := decls[key]; ok {
		return body, true
	}
	var found *ast.BlockStmt
	matches := 0
	for k, body := range decls {
		if strings.HasSuffix(k, "."+key) {
			found = body
			matches++
		}
	}
	if matches == 1 {
		return found, true
	}
	return nil, false
}

func callKey(e ast.Expr) string {
	switch v := e.(type) {
	case *ast.SelectorExpr:
		return v.Sel.Name
	case *ast.Ident:
		return v.Name
	}
	return ""
}

func hasRecoverCall(body *ast.BlockStmt) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		if id, ok := n.(*ast.Ident); ok && (id.Name == "recoverLog" || id.Name == "recover") {
			found = true
		}
		return true
	})
	return found
}

func nodeText(fset *token.FileSet, n ast.Node) string {
	start := fset.Position(n.Pos())
	end := fset.Position(n.End())
	if start.Filename != end.Filename || start.Offset < 0 {
		return ""
	}
	b, err := os.ReadFile(filepath.Base(start.Filename))
	if err != nil || end.Offset > len(b) {
		return ""
	}
	return string(b[start.Offset:end.Offset])
}
