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
// 未知一律报错，不静默跳过：跨包调用和函数变量看不到函数体，就必须在这里
// 登记并指向真实的函数，否则契约会留下一个「以后再说」的洞。
func TestGoroutinesRecoverPanics(t *testing.T) {
	// 只等子进程退出，不碰应用状态，panic 不可能发生。
	// 逐字比对：改写这处 goroutine 会让登记失效，逼审核者重新判断。
	allowedLiterals := map[string]string{
		"audio.go": "go func() { _ = c.Wait() }()",
	}

	// 包级函数变量：goroutine 里写的是变量名，实际函数指向这里。每条都要
	// 解析到同包函数并检查它的函数体，所以登记本身不会放过一个裸函数。
	functionVars := map[string]string{
		"OnMouseDown": "onGlobalMouseDown",
		"OnMouseMove": "onChipPointer",
		"OnKeyDown":   "onGlobalKeyDown",
	}

	// 跨包调用：看不到函数体，逐条登记理由。
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

			// 同包函数。
			if body, ok := lookup(decls, key); ok {
				if !hasRecoverCall(body) {
					t.Errorf("%s:%d 的 goroutine 目标函数 %s 没有 recoverLog：panic 会终止整个进程",
						name, line, key)
				}
				return true
			}

			// 函数变量：解析到它指向的同包函数，不能只信登记的名字。
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

			// 跨包调用：必须登记。
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

// lookup 按全名找，再按方法名找。调用处写的是接收者变量 a，类型却是 app，
// 两者对不上；同名方法只有一份时才认，有歧义就当找不到。
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

// callKey 把 a.rebuildTray / rebuildTray() / mygo.Shell.OpenExternal 归一成
// 方法名或函数名：接收者变量名与类型名不同，只能按最后一段匹配。
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

// nodeText 取出源码片段，用于白名单逐字比对。
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
