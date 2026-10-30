package config

import (
	"bytes"
	"errors"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const sampleYAML = `# 是否显示进度
print-progress: true

# 并发线程数（测活阶段使用此值）
concurrent: 50
# 测速并发数（默认等于concurrent）
speed-concurrent: 20
# 检查间隔(分钟)
check-interval: 120
# 超时时间(毫秒)(节点的最大延迟)
timeout: 5000

# 只测试指定协议的节点
node-type:
  # - ss
  # - vmess

# 节点名称过滤
filter:
  # === 按地区过滤 ===
  # - "香港|HK"

# 订阅地址
sub-urls:
  - https://example.com/sub.txt#备注

# 我自己加的键，程序不认识
my-own-note: 手工保留
`

func mustPatch(t *testing.T, src string, values map[string]any) string {
	t.Helper()
	got, err := PatchYAML([]byte(src), values)
	if err != nil {
		t.Fatalf("PatchYAML: %v", err)
	}
	return string(got)
}

// 改一个标量值：只动那一行，注释、未知键、键顺序都要留着。
func TestPatchYAML_ReplacesScalarAndKeepsEverythingElse(t *testing.T) {
	got := mustPatch(t, sampleYAML, map[string]any{"concurrent": 80})

	if !strings.Contains(got, "# 并发线程数（测活阶段使用此值）\nconcurrent: 80\n") {
		t.Fatalf("并发注释与其新值没有挨在一起:\n%s", got)
	}
	if strings.Contains(got, "concurrent: 50") {
		t.Fatal("旧值还在")
	}
	for _, want := range []string{
		"# 是否显示进度",
		"# 测速并发数（默认等于concurrent）",
		"# 只测试指定协议的节点",
		`# - "香港|HK"`,
		"my-own-note: 手工保留",
		"  - https://example.com/sub.txt#备注",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("丢了 %q:\n%s", want, got)
		}
	}

	// 键顺序：concurrent 仍在 speed-concurrent 之前、timeout 之后
	if strings.Index(got, "concurrent: 80") > strings.Index(got, "speed-concurrent: 20") {
		t.Fatal("键顺序被打乱")
	}
}

// 列表值整体替换，块后面的键不能跟着被吃掉。
func TestPatchYAML_ReplacesList(t *testing.T) {
	got := mustPatch(t, sampleYAML, map[string]any{
		"sub-urls": []string{"https://a.example/sub", "https://b.example/sub#机场B"},
	})

	want := "sub-urls:\n  - https://a.example/sub\n  - https://b.example/sub#机场B\n\n# 我自己加的键，程序不认识\nmy-own-note: 手工保留\n"
	if !strings.Contains(got, want) {
		t.Fatalf("列表块替换结果不对:\n%s", got)
	}
	if strings.Contains(got, "https://example.com/sub.txt") {
		t.Fatal("旧列表还在")
	}
}

// 文件里没有的键追加到末尾，且保留结尾换行。
func TestPatchYAML_AppendsMissingKey(t *testing.T) {
	got := mustPatch(t, sampleYAML, map[string]any{"min-speed": 512})

	if !strings.HasSuffix(got, "my-own-note: 手工保留\nmin-speed: 512\n") {
		t.Fatalf("新键没有追加到末尾:\n%s", got)
	}
}

// nil 删除键，连同它缩进的子块。
func TestPatchYAML_DeletesKeyAndItsBlock(t *testing.T) {
	got := mustPatch(t, sampleYAML, map[string]any{"node-type": nil})

	if strings.Contains(got, "node-type:") {
		t.Fatalf("键没被删掉:\n%s", got)
	}
	if strings.Contains(got, "# - vmess") {
		t.Fatalf("子块没被删掉:\n%s", got)
	}
	// 相邻块必须完好
	for _, want := range []string{"# 节点名称过滤", "filter:", `# - "香港|HK"`, "sub-urls:"} {
		if !strings.Contains(got, want) {
			t.Fatalf("误删了 %q:\n%s", want, got)
		}
	}
}

// 值没变时一个字节都不动，这样"保存"不会把用户文件摸一遍。
func TestPatchYAML_UnchangedValueIsByteIdentical(t *testing.T) {
	got := mustPatch(t, sampleYAML, map[string]any{"concurrent": 50, "timeout": 5000})
	if got != sampleYAML {
		t.Fatalf("无改动却重写了文件:\n%s", got)
	}
}

// 值没变时连引号风格也要保住：":8199" 不能被"优化"成 :8199。
func TestPatchYAML_UnchangedValueKeepsOriginalQuoting(t *testing.T) {
	src := "# 监听端口\nlisten-port: \":8199\"\n"
	got := mustPatch(t, src, map[string]any{"listen-port": ":8199"})
	if got != src {
		t.Fatalf("值相同却改了写法:\n got %q\nwant %q", got, src)
	}
}

// 列表块里的缩进注释是写给用户看的示例，换值时不能当垃圾丢掉。
func TestPatchYAML_ListReplacementKeepsInnerComments(t *testing.T) {
	src := `# 订阅地址
sub-urls:
  # - https://example.com/sub.txt
  # - https://example.com/sub2.txt
  - https://keep-or-replace.example/a

# 下一个键
keep-days: 0
`
	got := mustPatch(t, src, map[string]any{"sub-urls": []string{"https://new.example/sub#机场A"}})

	for _, want := range []string{
		"  # - https://example.com/sub.txt",
		"  # - https://example.com/sub2.txt",
		"  - https://new.example/sub#机场A",
		"# 下一个键",
		"keep-days: 0",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("丢了 %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "keep-or-replace") {
		t.Fatalf("旧值还在:\n%s", got)
	}
}

// 需要引号的值交给 YAML 编码器处理，并能原样解析回来。
func TestPatchYAML_QuotesValuesThatNeedIt(t *testing.T) {
	got := mustPatch(t, sampleYAML, map[string]any{
		"listen-port":  ":8199",
		"output-dir":   "",
		"notify-title": "🔔 节点状态更新",
	})

	var parsed map[string]any
	if err := yaml.Unmarshal([]byte(got), &parsed); err != nil {
		t.Fatalf("结果不是合法 YAML: %v\n%s", err, got)
	}
	if parsed["listen-port"] != ":8199" {
		t.Fatalf("listen-port = %#v", parsed["listen-port"])
	}
	if parsed["output-dir"] != "" {
		t.Fatalf("output-dir = %#v", parsed["output-dir"])
	}
	if parsed["notify-title"] != "🔔 节点状态更新" {
		t.Fatalf("notify-title = %#v", parsed["notify-title"])
	}
}

// 真实模板是最狠的回归用例：改动后仍能解析，注释一条不少。
func TestPatchYAML_KeepsRealTemplateComments(t *testing.T) {
	src := string(DefaultConfigTemplate)
	before := countCommentLines(src)

	got := mustPatch(t, src, map[string]any{
		"concurrent": 80,
		"min-speed":  1024,
		"sub-urls":   []string{"https://new.example/sub#机场A"},
		"platforms":  []string{"netflix", "openai", "iprisk"},
	})

	if n := countCommentLines(got); n != before {
		t.Fatalf("注释行数变了：%d → %d", before, n)
	}
	var parsed map[string]any
	if err := yaml.Unmarshal([]byte(got), &parsed); err != nil {
		t.Fatalf("结果不是合法 YAML: %v", err)
	}
	if parsed["concurrent"] != 80 {
		t.Fatalf("concurrent = %#v", parsed["concurrent"])
	}
	if parsed["min-speed"] != 1024 {
		t.Fatalf("min-speed = %#v", parsed["min-speed"])
	}
	// 其余键仍在：抽查一个列表键
	if _, ok := parsed["sub-urls"]; !ok {
		t.Fatal("sub-urls 丢了")
	}
}

// countCommentLines 只数真正的注释行，值里的 #（例如订阅 URL 的 #备注）不算。
func countCommentLines(text string) int {
	n := 0
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			n++
		}
	}
	return n
}

func TestPatchYAML_CRLFStaysCRLF(t *testing.T) {
	src := strings.ReplaceAll(sampleYAML, "\n", "\r\n")
	got := mustPatch(t, src, map[string]any{"concurrent": 80})

	if !strings.Contains(got, "concurrent: 80\r\n") {
		t.Fatalf("CRLF 没保住:\n%q", got)
	}
	if strings.Contains(strings.ReplaceAll(got, "\r\n", ""), "\n") {
		t.Fatal("混进了裸 \\n")
	}
}

func TestPatchYAML_RejectsBadRequests(t *testing.T) {
	t.Run("根节点不是映射", func(t *testing.T) {
		_, err := PatchYAML([]byte("- a\n- b\n"), map[string]any{"concurrent": 1})
		if !errors.Is(err, ErrNotMapping) {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("非法键名", func(t *testing.T) {
		_, err := PatchYAML([]byte(sampleYAML), map[string]any{"a b": 1})
		if !errors.Is(err, ErrBadKey) {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("源文件不是合法 YAML", func(t *testing.T) {
		_, err := PatchYAML([]byte("concurrent: [1, 2\n"), map[string]any{"concurrent": 1})
		if err == nil {
			t.Fatal("应当报错")
		}
	})
}

func TestPatchChanged_只报真正变了的键(t *testing.T) {
	changed, err := PatchChanged([]byte(sampleYAML), map[string]any{
		"concurrent": 80,   // 变
		"timeout":    5000, // 没变
		"min-speed":  512,  // 新增，算变
		"node-type":  nil,  // 删除，算变
		"filter":     nil,  // 删除，算变
		"keep-days":  nil,  // 本来就没有，不算
	})
	if err != nil {
		t.Fatalf("PatchChanged: %v", err)
	}
	want := []string{"concurrent", "filter", "min-speed", "node-type"}
	if !reflect.DeepEqual(changed, want) {
		t.Fatalf("changed = %v, want %v", changed, want)
	}
}

func TestPatchChanged_无改动返回空(t *testing.T) {
	changed, err := PatchChanged([]byte(sampleYAML), map[string]any{"concurrent": 50})
	if err != nil {
		t.Fatalf("PatchChanged: %v", err)
	}
	if len(changed) != 0 {
		t.Fatalf("changed = %v, want empty", changed)
	}
}

func TestRequiresRestart_保持输入顺序且只挑重启键(t *testing.T) {
	got := RequiresRestart([]string{"listen-port", "check-interval", "proxy", "concurrent"})
	want := []string{"listen-port", "proxy"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	if len(RequiresRestart(nil)) != 0 {
		t.Fatal("nil 应当返回空")
	}
}

// 无改动时不得给调用方一个假备份路径。
func TestPatchYAML_EmptyValuesIsNoop(t *testing.T) {
	got, err := PatchYAML([]byte(sampleYAML), nil)
	if err != nil {
		t.Fatalf("PatchYAML: %v", err)
	}
	if !bytes.Equal(got, []byte(sampleYAML)) {
		t.Fatal("空请求改动了文件")
	}
}
