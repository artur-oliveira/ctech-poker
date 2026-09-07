package table

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

// deliberateStaleCacheTimerHandlers are the timer-fired handlers knowingly left
// on ensureLoaded(ctx, false) by docs/specs/2026-09-04-cross-instance-stale-turn-timer.md.
// Removing a name from this map is a fix; ADDING one is a decision that needs
// the same written reasoning, not a way to make this test pass.
var deliberateStaleCacheTimerHandlers = map[string]string{
	"handleKickTimeout":       "removal is justified from persisted LastActionAt, not from the cached snapshot",
	"handleAFKSweep":          "same as handleKickTimeout — the sweep re-reads what it acts on",
	"handleExpireWinnerCards": "expiry only clears an offer; a stale cache costs a no-op, never a wrong charge",
}

// TestTimerFiredHandlersForceReload is issue #370's guard: trustCache is
// per-handler opt-in, and the stale-cache class of bug it enables has already
// been found twice (seating, timers). A handler reached from a time.AfterFunc
// can run minutes after another instance carried the hand forward — tablelease
// is latency-only, so several instances run their own Actor for one table —
// and ensureLoaded(ctx, false)'s trustCache fast path lets that stale snapshot
// pass the handler's own current-player/stage guard.
//
// Rather than a review checklist nobody runs, this derives the set from the
// source itself: every command type dispatched from inside a time.AfterFunc,
// mapped through handle's type switch to its handler, must call
// ensureLoaded(..., true). A new background handler added later is therefore
// caught here, not in production.
func TestTimerFiredHandlersForceReload(t *testing.T) {
	// The package's own non-test sources, parsed directly: go/parser.ParseDir
	// is deprecated, and golang.org/x/tools/go/packages would pull a
	// dependency for a scan that needs nothing but the syntax.
	paths, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	fset := token.NewFileSet()
	files := make([]*ast.File, 0, len(paths))
	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		file, parseErr := parser.ParseFile(fset, path, nil, 0)
		if parseErr != nil {
			t.Fatalf("parse %s: %v", path, parseErr)
		}
		files = append(files, file)
	}
	if len(files) == 0 {
		t.Fatal("no package sources found — the scan itself broke")
	}

	timerCommands := map[string]bool{}
	handlerForCommand := map[string]string{}
	handlerBodies := map[string]*ast.FuncDecl{}

	for _, file := range files {
		ast.Inspect(file, func(n ast.Node) bool {
			switch node := n.(type) {
			case *ast.CallExpr:
				if selectorName(node.Fun) == "time.AfterFunc" {
					for _, cmd := range dispatchedCommands(node) {
						timerCommands[cmd] = true
					}
				}
			case *ast.TypeSwitchStmt:
				for _, clause := range node.Body.List {
					caseClause, ok := clause.(*ast.CaseClause)
					if !ok || len(caseClause.List) != 1 {
						continue
					}
					ident, ok := caseClause.List[0].(*ast.Ident)
					if !ok {
						continue
					}
					for _, handler := range calledMethods(caseClause) {
						if strings.HasPrefix(handler, "handle") {
							handlerForCommand[ident.Name] = handler
						}
					}
				}
			case *ast.FuncDecl:
				if node.Recv != nil {
					handlerBodies[node.Name.Name] = node
				}
			}
			return true
		})
	}

	if len(timerCommands) == 0 {
		t.Fatal("found no time.AfterFunc-dispatched commands — the scan itself broke")
	}

	checked := 0
	for cmd := range timerCommands {
		handler, ok := handlerForCommand[cmd]
		if !ok {
			t.Fatalf("%s is dispatched from a timer but has no handler in handle's type switch", cmd)
		}
		if reason, deliberate := deliberateStaleCacheTimerHandlers[handler]; deliberate {
			t.Logf("%s: deliberately left on trustCache (%s)", handler, reason)
			continue
		}
		decl, ok := handlerBodies[handler]
		if !ok {
			t.Fatalf("handler %s not found", handler)
		}
		checked++
		if !forcesReload(decl) {
			t.Errorf("%s is reached from a time.AfterFunc and must call ensureLoaded(ctx, true): a timer can "+
				"fire long after another instance carried the hand forward, and trustCache would let that stale "+
				"snapshot pass the handler's own stage guard (issue #370)", handler)
		}
	}
	// The three handlers fixed by the 2026-09-04 incident. Fewer than that
	// means the scan stopped finding them, not that the code got safer.
	if checked < 3 {
		t.Fatalf("only %d timer-fired handlers were checked — the scan missed some", checked)
	}
}

// forcesReload reports whether decl calls ensureLoaded with a literal true and
// never with a literal false — one forced reload does not help if another
// branch of the same handler re-reads through the cache.
func forcesReload(decl *ast.FuncDecl) bool {
	forced, trusted := false, false
	ast.Inspect(decl, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || !strings.HasSuffix(selectorName(call.Fun), ".ensureLoaded") || len(call.Args) != 2 {
			return true
		}
		switch literalName(call.Args[1]) {
		case "true":
			forced = true
		case "false":
			trusted = true
		}
		return true
	})
	return forced && !trusted
}

// dispatchedCommands returns the command type names constructed inside a
// call's function-literal argument, e.g. a.Dispatch(turnTimeoutCmd{...}).
func dispatchedCommands(call *ast.CallExpr) []string {
	var names []string
	ast.Inspect(call, func(n ast.Node) bool {
		inner, ok := n.(*ast.CallExpr)
		if !ok || !strings.HasSuffix(selectorName(inner.Fun), ".Dispatch") {
			return true
		}
		for _, arg := range inner.Args {
			composite, ok := arg.(*ast.CompositeLit)
			if !ok {
				continue
			}
			if ident, ok := composite.Type.(*ast.Ident); ok {
				names = append(names, ident.Name)
			}
		}
		return true
	})
	return names
}

func calledMethods(node ast.Node) []string {
	var names []string
	ast.Inspect(node, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok {
			if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
				names = append(names, sel.Sel.Name)
			}
		}
		return true
	})
	return names
}

func selectorName(expr ast.Expr) string {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return ""
	}
	if ident, ok := sel.X.(*ast.Ident); ok {
		return ident.Name + "." + sel.Sel.Name
	}
	return "." + sel.Sel.Name
}

func literalName(expr ast.Expr) string {
	if ident, ok := expr.(*ast.Ident); ok {
		return ident.Name
	}
	return ""
}
