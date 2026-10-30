package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

const settingsFixture = `# 是否显示进度
print-progress: true
# 并发线程数（测活阶段使用此值）
concurrent: 50
# 监听端口
listen-port: ":8199"
# 我自己的键
my-own-note: 别动我
`

func newSettingsFixture(t *testing.T) (string, *App) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(settingsFixture), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return path, &App{configPath: path}
}

func TestPatchConfig_WritesBackupAndKeepsComments(t *testing.T) {
	path, app := newSettingsFixture(t)

	res, err := app.patchConfig(map[string]any{"concurrent": 80})
	if err != nil {
		t.Fatalf("patchConfig: %v", err)
	}
	if !reflect.DeepEqual(res.Changed, []string{"concurrent"}) {
		t.Fatalf("changed = %v", res.Changed)
	}
	if len(res.NeedsRestart) != 0 {
		t.Fatalf("needsRestart = %v, want empty", res.NeedsRestart)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !strings.Contains(string(got), "# 并发线程数（测活阶段使用此值）\nconcurrent: 80\n") {
		t.Fatalf("改写结果不对:\n%s", got)
	}
	if !strings.Contains(string(got), "my-own-note: 别动我") {
		t.Fatalf("未知键丢了:\n%s", got)
	}

	backup, err := os.ReadFile(res.BackupPath)
	if err != nil {
		t.Fatalf("备份读不到: %v", err)
	}
	if string(backup) != settingsFixture {
		t.Fatalf("备份内容应等于改动前原文:\n%s", backup)
	}
}

func TestPatchConfig_FlagsRestartOnlyKeys(t *testing.T) {
	_, app := newSettingsFixture(t)

	res, err := app.patchConfig(map[string]any{"listen-port": ":9000", "concurrent": 100})
	if err != nil {
		t.Fatalf("patchConfig: %v", err)
	}
	if !reflect.DeepEqual(res.NeedsRestart, []string{"listen-port"}) {
		t.Fatalf("needsRestart = %v, want [listen-port]", res.NeedsRestart)
	}
}

func TestPatchConfig_NoChangeWritesNothing(t *testing.T) {
	path, app := newSettingsFixture(t)

	res, err := app.patchConfig(map[string]any{"concurrent": 50})
	if err != nil {
		t.Fatalf("patchConfig: %v", err)
	}
	if len(res.Changed) != 0 || res.BackupPath != "" {
		t.Fatalf("res = %+v, want 空结果", res)
	}
	if _, err := os.Stat(path + ".bak"); !os.IsNotExist(err) {
		t.Fatal("无改动时不该留备份")
	}
}

func TestUpdateSettings_HTTP(t *testing.T) {
	_, app := newSettingsFixture(t)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/settings", app.updateSettings)

	body := `{"concurrent":80,"listen-port":":9000"}`
	req := httptest.NewRequest(http.MethodPost, "/api/settings", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var got struct {
		Changed      []string `json:"changed"`
		NeedsRestart []string `json:"needsRestart"`
		Backup       string   `json:"backup"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("bad json: %v", err)
	}
	if !reflect.DeepEqual(got.Changed, []string{"concurrent", "listen-port"}) {
		t.Fatalf("changed = %v", got.Changed)
	}
	if !reflect.DeepEqual(got.NeedsRestart, []string{"listen-port"}) {
		t.Fatalf("needsRestart = %v", got.NeedsRestart)
	}
	if got.Backup == "" {
		t.Fatal("backup 路径为空")
	}
}

func TestUpdateSettings_RejectsBadInput(t *testing.T) {
	_, app := newSettingsFixture(t)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/settings", app.updateSettings)

	for name, body := range map[string]string{
		"坏 JSON": `{`,
		"空对象":    `{}`,
		"非法键名":   `{"a b": 1}`,
	} {
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/settings", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			if w.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestGetConfig_ReturnsParsedValues(t *testing.T) {
	_, app := newSettingsFixture(t)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/api/config", app.getConfig)

	req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var got struct {
		Content string         `json:"content"`
		Values  map[string]any `json:"values"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("bad json: %v", err)
	}
	if !strings.Contains(got.Content, "# 并发线程数") {
		t.Fatalf("content 应保留注释:\n%s", got.Content)
	}
	// 表单要按值回填，键名与 config.yaml 一致
	if got.Values["concurrent"] != float64(50) {
		t.Fatalf("values[concurrent] = %v, want 50", got.Values["concurrent"])
	}
	if got.Values["listen-port"] != ":8199" {
		t.Fatalf("values[listen-port] = %v", got.Values["listen-port"])
	}
	if got.Values["my-own-note"] != "别动我" {
		t.Fatalf("表单不认识的键也该原样给出: %v", got.Values["my-own-note"])
	}
}

func TestGetConfig_BadYAMLOmitsValues(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("concurrent: [unclosed\n"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	app := &App{configPath: path}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/api/config", app.getConfig)

	req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var got struct {
		Content string         `json:"content"`
		Values  map[string]any `json:"values"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("bad json: %v", err)
	}
	if got.Content == "" {
		t.Fatal("YAML 坏了也要把原文交给编辑器")
	}
	if got.Values != nil {
		t.Fatalf("YAML 坏了不该给 values: %v", got.Values)
	}
}
