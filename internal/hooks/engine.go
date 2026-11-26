package hooks

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/dop251/goja"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v4"
)

const hookTimeout = 5 * time.Second

type hookHandler struct {
	collection string
	fn         goja.Callable
	file       string
}

type customRoute struct {
	method string
	path   string
	fn     goja.Callable
	file   string
}

// Engine manages embedded JavaScript hooks (mb_hooks/*.js) using Goja.
type Engine struct {
	mu     sync.Mutex
	vm     *goja.Runtime
	pool   *pgxpool.Pool
	hooks  map[string][]hookHandler // event -> handlers
	routes []customRoute
	files  []string
}

// NewEngine creates a new JS hooks engine bound to the optional PostgreSQL pool.
func NewEngine(pool *pgxpool.Pool) *Engine {
	vm := goja.New()
	eng := &Engine{
		vm:    vm,
		pool:  pool,
		hooks: make(map[string][]hookHandler),
	}
	eng.registerGlobals()
	return eng
}

// LoadDir loads and executes all *.js files in dir in sorted order.
// Missing directories are ignored cleanly.
func (e *Engine) LoadDir(dir string) error {
	if strings.TrimSpace(dir) == "" {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read hooks dir %s: %w", dir, err)
	}
	var jsFiles []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(strings.ToLower(entry.Name()), ".js") {
			jsFiles = append(jsFiles, entry.Name())
		}
	}
	sort.Strings(jsFiles)

	for _, name := range jsFiles {
		full := filepath.Join(dir, name)
		src, err := os.ReadFile(full)
		if err != nil {
			return fmt.Errorf("read hook %s: %w", name, err)
		}
		if err := e.LoadScript(name, string(src)); err != nil {
			return err
		}
	}
	if len(jsFiles) > 0 {
		slog.Info("js hooks loaded", "dir", dir, "files", len(jsFiles), "routes", len(e.routes))
	}
	return nil
}

// LoadScript compiles and runs a JS hook script inside the engine.
func (e *Engine) LoadScript(name, src string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.vm.Set("__currentHookFile", name)
	timer := time.AfterFunc(hookTimeout, func() {
		e.vm.Interrupt("hook initialization timed out")
	})
	defer timer.Stop()
	defer e.vm.ClearInterrupt()

	if _, err := e.vm.RunScript(name, src); err != nil {
		return fmt.Errorf("hook %s failed: %w", name, err)
	}
	e.files = append(e.files, name)
	return nil
}

// RegisterRoutes mounts any custom routes declared via routerAdd(method, path, handler) onto Echo.
func (e *Engine) RegisterRoutes(ec *echo.Echo) {
	e.mu.Lock()
	routesCopy := append([]customRoute(nil), e.routes...)
	e.mu.Unlock()

	for _, r := range routesCopy {
		route := r
		handler := func(c echo.Context) error {
			return e.invokeRoute(route, c)
		}
		switch strings.ToUpper(route.method) {
		case http.MethodGet:
			ec.GET(route.path, handler)
		case http.MethodPost:
			ec.POST(route.path, handler)
		case http.MethodPut:
			ec.PUT(route.path, handler)
		case http.MethodPatch:
			ec.PATCH(route.path, handler)
		case http.MethodDelete:
			ec.DELETE(route.path, handler)
		default:
			ec.Any(route.path, handler)
		}
		slog.Info("js hook route mounted", "method", route.method, "path", route.path, "file", route.file)
	}
}

func (e *Engine) invokeRoute(route customRoute, c echo.Context) error {
	var bodyMap map[string]any
	if c.Request().Body != nil {
		raw, _ := io.ReadAll(io.LimitReader(c.Request().Body, 2<<20))
		if len(raw) > 0 {
			_ = json.Unmarshal(raw, &bodyMap)
		}
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	timer := time.AfterFunc(hookTimeout, func() {
		e.vm.Interrupt("route handler timed out")
	})
	defer timer.Stop()
	defer e.vm.ClearInterrupt()

	statusCode := http.StatusOK
	ctxObj := e.vm.NewObject()
	_ = ctxObj.Set("method", c.Request().Method)
	_ = ctxObj.Set("path", c.Request().URL.Path)
	_ = ctxObj.Set("body", bodyMap)
	_ = ctxObj.Set("param", func(name string) string { return c.Param(name) })
	_ = ctxObj.Set("query", func(name string) string { return c.QueryParam(name) })
	_ = ctxObj.Set("header", func(name string) string { return c.Request().Header.Get(name) })
	_ = ctxObj.Set("status", func(code int) { statusCode = code })

	val, err := route.fn(goja.Undefined(), ctxObj)
	if err != nil {
		slog.Error("js route error", "path", route.path, "file", route.file, "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]any{"error": err.Error()})
	}
	if val == nil || goja.IsUndefined(val) || goja.IsNull(val) {
		return c.NoContent(statusCode)
	}
	return c.JSON(statusCode, val.Export())
}

// RunBefore executes before-lifecycle hooks (e.g. "beforeCreate", "beforeUpdate", "beforeDelete").
// Hooks may mutate payload["data"] in place or throw an Error to abort the operation.
func (e *Engine) RunBefore(event, collection string, payload map[string]any) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	handlers := e.matchingHandlers(event, collection)
	if len(handlers) == 0 {
		return nil
	}

	for _, h := range handlers {
		timer := time.AfterFunc(hookTimeout, func() {
			e.vm.Interrupt("hook execution timed out")
		})
		evVal := e.vm.ToValue(payload)
		_, err := h.fn(goja.Undefined(), evVal)
		timer.Stop()
		e.vm.ClearInterrupt()
		if err != nil {
			if ex, ok := err.(*goja.Exception); ok {
				return fmt.Errorf("%s", strings.TrimPrefix(ex.Value().String(), "Error: "))
			}
			return err
		}
		// Copy back any mutated fields in payload["data"]
		if exported, ok := evVal.Export().(map[string]any); ok {
			if newData, ok := exported["data"].(map[string]any); ok {
				if origData, ok := payload["data"].(map[string]any); ok {
					for k, v := range newData {
						origData[k] = v
					}
				}
			}
		}
	}
	return nil
}

// RunAfter executes after-lifecycle hooks asynchronously/safely (errors are logged, never fail the request).
func (e *Engine) RunAfter(event, collection string, payload map[string]any) {
	e.mu.Lock()
	defer e.mu.Unlock()

	handlers := e.matchingHandlers(event, collection)
	if len(handlers) == 0 {
		return
	}

	for _, h := range handlers {
		timer := time.AfterFunc(hookTimeout, func() {
			e.vm.Interrupt("after-hook timed out")
		})
		_, err := h.fn(goja.Undefined(), e.vm.ToValue(payload))
		timer.Stop()
		e.vm.ClearInterrupt()
		if err != nil {
			slog.Warn("js after-hook error", "event", event, "collection", collection, "file", h.file, "error", err)
		}
	}
}

func (e *Engine) matchingHandlers(event, collection string) []hookHandler {
	list := e.hooks[event]
	if len(list) == 0 {
		return nil
	}
	var out []hookHandler
	for _, h := range list {
		if h.collection == "*" || h.collection == "" || h.collection == collection {
			out = append(out, h)
		}
	}
	return out
}

func (e *Engine) registerGlobals() {
	registerHook := func(event string) func(goja.FunctionCall) goja.Value {
		return func(call goja.FunctionCall) goja.Value {
			var col string
			var fnVal goja.Value
			if len(call.Arguments) == 1 {
				col = "*"
				fnVal = call.Arguments[0]
			} else if len(call.Arguments) >= 2 {
				col = call.Arguments[0].String()
				fnVal = call.Arguments[1]
			} else {
				panic(e.vm.NewTypeError("hook requires (collection, fn) or (fn)"))
			}
			fn, ok := goja.AssertFunction(fnVal)
			if !ok {
				panic(e.vm.NewTypeError("hook callback must be a function"))
			}
			curFile := ""
			if fv := e.vm.Get("__currentHookFile"); fv != nil {
				curFile = fv.String()
			}
			e.hooks[event] = append(e.hooks[event], hookHandler{collection: col, fn: fn, file: curFile})
			return goja.Undefined()
		}
	}

	_ = e.vm.Set("onRecordBeforeCreate", registerHook("beforeCreate"))
	_ = e.vm.Set("onRecordAfterCreate", registerHook("afterCreate"))
	_ = e.vm.Set("onRecordBeforeUpdate", registerHook("beforeUpdate"))
	_ = e.vm.Set("onRecordAfterUpdate", registerHook("afterUpdate"))
	_ = e.vm.Set("onRecordBeforeDelete", registerHook("beforeDelete"))
	_ = e.vm.Set("onRecordAfterDelete", registerHook("afterDelete"))

	_ = e.vm.Set("routerAdd", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 3 {
			panic(e.vm.NewTypeError("routerAdd requires (method, path, handler)"))
		}
		method := strings.ToUpper(call.Arguments[0].String())
		path := call.Arguments[1].String()
		fn, ok := goja.AssertFunction(call.Arguments[2])
		if !ok {
			panic(e.vm.NewTypeError("routerAdd handler must be a function"))
		}
		curFile := ""
		if fv := e.vm.Get("__currentHookFile"); fv != nil {
			curFile = fv.String()
		}
		e.routes = append(e.routes, customRoute{method: method, path: path, fn: fn, file: curFile})
		return goja.Undefined()
	})

	// console.log / warn / error
	consoleObj := e.vm.NewObject()
	formatArgs := func(args []goja.Value) string {
		parts := make([]string, len(args))
		for i, a := range args {
			parts[i] = a.String()
		}
		return strings.Join(parts, " ")
	}
	_ = consoleObj.Set("log", func(call goja.FunctionCall) goja.Value {
		slog.Info("[hook] " + formatArgs(call.Arguments))
		return goja.Undefined()
	})
	_ = consoleObj.Set("warn", func(call goja.FunctionCall) goja.Value {
		slog.Warn("[hook] " + formatArgs(call.Arguments))
		return goja.Undefined()
	})
	_ = consoleObj.Set("error", func(call goja.FunctionCall) goja.Value {
		slog.Error("[hook] " + formatArgs(call.Arguments))
		return goja.Undefined()
	})
	_ = e.vm.Set("console", consoleObj)

	// $http.send({ url, method, headers, body, timeout })
	httpObj := e.vm.NewObject()
	_ = httpObj.Set("send", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			panic(e.vm.NewTypeError("$http.send requires an options object"))
		}
		opts, ok := call.Arguments[0].Export().(map[string]any)
		if !ok {
			panic(e.vm.NewTypeError("$http.send options must be an object"))
		}
		rawURL := fmt.Sprint(opts["url"])
		method := "GET"
		if m, ok := opts["method"].(string); ok && m != "" {
			method = strings.ToUpper(m)
		}
		var bodyReader io.Reader
		if b, exists := opts["body"]; exists && b != nil {
			switch bv := b.(type) {
			case string:
				bodyReader = strings.NewReader(bv)
			default:
				jb, _ := json.Marshal(bv)
				bodyReader = bytes.NewReader(jb)
			}
		}
		req, err := http.NewRequest(method, rawURL, bodyReader)
		if err != nil {
			panic(e.vm.NewGoError(err))
		}
		if hdrs, ok := opts["headers"].(map[string]any); ok {
			for k, v := range hdrs {
				req.Header.Set(k, fmt.Sprint(v))
			}
		}
		resp, err := (&http.Client{Timeout: 4 * time.Second}).Do(req)
		if err != nil {
			panic(e.vm.NewGoError(err))
		}
		defer resp.Body.Close()
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
		var jsonBody any
		_ = json.Unmarshal(respBody, &jsonBody)
		return e.vm.ToValue(map[string]any{
			"statusCode": resp.StatusCode,
			"raw":        string(respBody),
			"json":       jsonBody,
		})
	})
	_ = e.vm.Set("$http", httpObj)

	// $db.query(sql, ...args) & $db.exec(sql, ...args)
	dbObj := e.vm.NewObject()
	_ = dbObj.Set("query", func(call goja.FunctionCall) goja.Value {
		if e.pool == nil || len(call.Arguments) < 1 {
			return e.vm.ToValue([]any{})
		}
		sqlStr := call.Arguments[0].String()
		args := make([]any, 0, len(call.Arguments)-1)
		for _, a := range call.Arguments[1:] {
			args = append(args, a.Export())
		}
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		defer cancel()
		rows, err := e.pool.Query(ctx, sqlStr, args...)
		if err != nil {
			panic(e.vm.NewGoError(err))
		}
		defer rows.Close()
		fds := rows.FieldDescriptions()
		var out []map[string]any
		for rows.Next() {
			vals, err := rows.Values()
			if err != nil {
				continue
			}
			row := make(map[string]any, len(fds))
			for i, fd := range fds {
				row[string(fd.Name)] = vals[i]
			}
			out = append(out, row)
		}
		return e.vm.ToValue(out)
	})
	_ = dbObj.Set("exec", func(call goja.FunctionCall) goja.Value {
		if e.pool == nil || len(call.Arguments) < 1 {
			return e.vm.ToValue(0)
		}
		sqlStr := call.Arguments[0].String()
		args := make([]any, 0, len(call.Arguments)-1)
		for _, a := range call.Arguments[1:] {
			args = append(args, a.Export())
		}
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		defer cancel()
		tag, err := e.pool.Exec(ctx, sqlStr, args...)
		if err != nil {
			panic(e.vm.NewGoError(err))
		}
		return e.vm.ToValue(tag.RowsAffected())
	})
	_ = e.vm.Set("$db", dbObj)
}
