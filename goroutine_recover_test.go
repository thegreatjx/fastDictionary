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

// 后台 goroutine 必须自己 recover，这条约定写在 logging.go 的 recoverLog 上：
// windowsgui 构建没有控制台，进程被 panic 终止时日志也留不下来。
//
// 之前没有任何机制守住它，热键回调、钩子命令队列、鼠标与按键回调、托盘重建
// 都是裸 goroutine，一次 panic 就带走整个托盘进程。这个测试把约定变成可执行
// 契约：新增裸 goroutine 会立刻变红，而不是等用户报告「取词突然失效」。
//
// 判定沿用 recoverLog 注释里的范围：goroutine 体内的 panic 只能靠它兜住。
// 同包内有函数体的调用会跟进去看，其它包（mygo、标准库）不追。
func TestGoroutinesRecoverPanics(t *testing.T) {
	// audio.go 的这一处只是等子进程退出，不接触任何应用状态，panic 不可能发生。
	allowed := map[string]string{
		"audio.go": "go func() { _ = c.Wait() }()",
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
			key := fd.Name.Name
			if fd.Recv != nil && len(fd.Recv.List) > 0 {
				key = methodKey(fd)
			}
			decls[key] = fd.Body
		}
	}

	for name, f := range files {
		ast.Inspect(f, func(n ast.Node) bool {
			g, ok := n.(*ast.GoStmt)
			if !ok {
				return true
			}
			pos := fset.Position(g.Pos())

			if lit, ok := g.Call.Fun.(*ast.FuncLit); ok {
				if lit.Body != nil && !hasRecoverCall(lit.Body) {
					if want, ok := allowed[name]; ok && nodeText(fset, f, g) == want {
						return true
					}
					t.Errorf("%s:%d 启动了裸 goroutine：panic 会终止整个进程，给它加 defer recoverLog(...)",
						name, pos.Line)
				}
				return true
			}

			// 同包方法调用：跟进去看函数体，跨包调用不追。
			if body := decls[callKey(g.Call.Fun)]; body != nil && !hasRecoverCall(body) {
				t.Errorf("%s:%d 的 goroutine 目标函数没有 recoverLog：panic 会终止整个进程",
					name, pos.Line)
			}
			return true
		})
	}
}

func methodKey(fd *ast.FuncDecl) string {
	t := fd.Recv.List[0].Type
	if star, ok := t.(*ast.StarExpr); ok {
		t = star.X
	}
	if id, ok := t.(*ast.Ident); ok {
		return id.Name + "." + fd.Name.Name
	}
	return fd.Name.Name
}

// callKey 把 a.rebuildTray / rebuildTray() 归一成 decls 里的键。
func callKey(e ast.Expr) string {
	switch v := e.(type) {
	case *ast.SelectorExpr:
		return v.Sel.Name // 接收者变量是 a，类型是 app，按方法名匹配
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

// nodeText 取出源码片段，用于白名单逐字比对：改写那处 goroutine 会让白名单失效，
// 迫使审核者重新判断，而不是被一条模糊的文件名规则永久放行。
func nodeText(fset *token.FileSet, f *ast.File, n ast.Node) string {
	start := fset.Position(n.Pos())
	end := fset.Position(n.End())
	if start.Filename != end.Filename {
		return ""
	}
	b, err := os.ReadFile(filepath.Base(start.Filename))
	if err != nil {
		return ""
	}
	if start.Offset < 0 || end.Offset > len(b) {
		return ""
	}
	return string(b[start.Offset:end.Offset])
}
