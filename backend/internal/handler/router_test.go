package handler_test

import (
	"context"
	"encoding/json"
	"go/parser"
	"go/token"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/smartinbox17-droid/canvas/backend/internal/database"
	"github.com/smartinbox17-droid/canvas/backend/internal/handler"
	"github.com/smartinbox17-droid/canvas/backend/internal/kernel"
	"github.com/smartinbox17-droid/canvas/backend/internal/outbound"
	"github.com/smartinbox17-droid/canvas/backend/internal/platform"
	"github.com/smartinbox17-droid/canvas/backend/internal/service"
)

func TestHealthAndEnvelopeContract(t *testing.T) {
	cfg := platform.Config{
		DatabaseDriver:      "sqlite",
		DataDir:             t.TempDir(),
		AutoMigrate:         true,
		RegistrationEnabled: true,
		CORSOrigins:         []string{"http://localhost:3000"},
	}
	db, err := database.Open(cfg)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	coord := platform.NewMemoryCoordinator()
	app, err := service.NewApplication(cfg, db, coord)
	if err != nil {
		t.Fatalf("new app: %v", err)
	}
	defer func() { _ = app.Close() }()

	router := handler.New(app).NewRouter()

	for _, path := range []string{"/api/health/live", "/api/health/startup", "/api/health/ready", "/api/v1/auth/settings", "/api/v1/auth/session"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s returned status %d: %s", path, rec.Code, rec.Body.String())
		}
		var env kernel.Envelope[map[string]any]
		if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
			t.Fatalf("GET %s invalid JSON envelope: %v", path, err)
		}
		if env.Code != 0 || env.Msg != "ok" || env.Reason != "" {
			t.Fatalf("GET %s unexpected envelope: %+v", path, env)
		}
	}

	req404 := httptest.NewRequest(http.MethodGet, "/api/v1/nonexistent", nil)
	rec404 := httptest.NewRecorder()
	router.ServeHTTP(rec404, req404)
	if rec404.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec404.Code)
	}
	var env404 kernel.Envelope[any]
	if err := json.Unmarshal(rec404.Body.Bytes(), &env404); err != nil {
		t.Fatalf("invalid 404 envelope: %v", err)
	}
	if env404.Reason != kernel.ReasonNotFound {
		t.Fatalf("expected reason NOT_FOUND, got %s", env404.Reason)
	}
}

func TestMemoryCoordinatorRateLimitAndSlots(t *testing.T) {
	ctx := context.Background()
	c := platform.NewMemoryCoordinator()

	ok1, _ := c.AllowFixedWindow(ctx, "u1", 2, time.Minute)
	ok2, _ := c.AllowFixedWindow(ctx, "u1", 2, time.Minute)
	ok3, _ := c.AllowFixedWindow(ctx, "u1", 2, time.Minute)
	if !ok1 || !ok2 || ok3 {
		t.Fatalf("unexpected rate limit results: %v %v %v", ok1, ok2, ok3)
	}

	s1, _ := c.AcquireSlot(ctx, "worker", "w1", 1, time.Minute)
	s2, _ := c.AcquireSlot(ctx, "worker", "w2", 1, time.Minute)
	if !s1 || s2 {
		t.Fatalf("expected slot capacity=1 enforcement: s1=%v s2=%v", s1, s2)
	}
	_ = c.ReleaseSlot(ctx, "worker", "w1")
	s3, _ := c.AcquireSlot(ctx, "worker", "w2", 1, time.Minute)
	if !s3 {
		t.Fatalf("expected slot acquire after release")
	}
}

func TestOutboundSSRFGuard(t *testing.T) {
	if !outbound.IsDisallowedIP(net.ParseIP("127.0.0.1")) {
		t.Fatal("127.0.0.1 must be disallowed")
	}
	if !outbound.IsDisallowedIP(net.ParseIP("10.0.0.1")) {
		t.Fatal("10.0.0.1 must be disallowed")
	}
	if !outbound.IsDisallowedIP(net.ParseIP("169.254.169.254")) {
		t.Fatal("169.254.169.254 link-local metadata IP must be disallowed")
	}
	if outbound.IsDisallowedIP(net.ParseIP("8.8.8.8")) {
		t.Fatal("8.8.8.8 public IP must be allowed")
	}
}

func TestArchitectureLayering(t *testing.T) {
	// Guard rule: domain/foundation packages under internal/{kernel,platform,outbound,model,database,repository}
	// must NEVER import internal/app, internal/service, or internal/handler.
	forbidden := []string{
		"github.com/smartinbox17-droid/canvas/backend/internal/app",
		"github.com/smartinbox17-droid/canvas/backend/internal/service",
		"github.com/smartinbox17-droid/canvas/backend/internal/handler",
	}
	pkgs := []string{"../kernel", "../platform", "../outbound", "../model", "../database", "../repository"}
	fset := token.NewFileSet()

	for _, dir := range pkgs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("read dir %s: %v", dir, err)
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") {
				continue
			}
			full := filepath.Join(dir, e.Name())
			f, err := parser.ParseFile(fset, full, nil, parser.ImportsOnly)
			if err != nil {
				t.Fatalf("parse %s: %v", full, err)
			}
			for _, imp := range f.Imports {
				impPath := strings.Trim(imp.Path.Value, `"`)
				for _, bad := range forbidden {
					if impPath == bad {
						t.Fatalf("layer violation: %s imports %s", full, impPath)
					}
				}
			}
		}
	}
}
