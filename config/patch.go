package config

import (
	"bytes"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// ErrNotMapping 表示配置文件根节点不是映射，无法按键改写。
var ErrNotMapping = errors.New("配置文件根节点不是映射")

// ErrBadKey 表示请求改写的键名不是合法的顶层键。
var ErrBadKey = errors.New("非法的配置键名")

// PatchResult 描述一次写回的结果。
type PatchResult struct {
	// Changed 是真正被改写（或删除）的键，按字母序；提交了但值没变的键不在此列。
	Changed []string
	// NeedsRestart 是 Changed 中需要重启进程才生效的键，保持 Changed 的顺序。
	NeedsRestart []string
	// BackupPath 是改动前的备份文件路径，没有改动时为空。
	BackupPath string
}

// restartOnlyKeys 列出只有重启进程才生效的键。
// 前四个取自 config.example.yaml 中标注「更新需重启程序」的键；后两个是启动时
// 才注册的路由与认证中间件（见 app.newRouter），热重载改不动它们。
var restartOnlyKeys = map[string]bool{
	"listen-port":    true,
	"sub-store-port": true,
	"sub-store-path": true,
	"proxy":          true,
	"enable-web-ui":  true,
	"api-key":        true,
}

// RequiresRestart 返回 keys 中需要重启才生效的那些，保持输入顺序。
func RequiresRestart(keys []string) []string {
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		if restartOnlyKeys[k] {
			out = append(out, k)
		}
	}
	return out
}

// keyPattern 限定能安全做行级替换的顶层键；带引号或空格键名会走 YAML 解析器
// 的另一条路径，行级匹配找不到它们，所以直接拒绝。
var keyPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

// PatchYAML 只改写 values 里给出的顶层键，其余字节原样保留：注释、程序不认识的
// 键、键顺序、空行都不动。值为 nil 表示删除该键（连同它缩进的子块）。
// 文件里不存在的键追加到末尾。
func PatchYAML(src []byte, values map[string]any) ([]byte, error) {
	if len(values) == 0 {
		return src, nil
	}
	for k := range values {
		if !keyPattern.MatchString(k) {
			return nil, fmt.Errorf("%w: %q", ErrBadKey, k)
		}
	}
	if err := checkMapping(src); err != nil {
		return nil, err
	}

	text := string(src)
	eol := "\n"
	if strings.Contains(text, "\r\n") {
		eol = "\r\n"
		text = strings.ReplaceAll(text, "\r\n", "\n")
	}
	lines := strings.Split(text, "\n")
	// 摘掉文件末尾换行产生的空元素，最后按需要补回。
	trailingNewline := false
	if n := len(lines); n > 0 && lines[n-1] == "" {
		lines = lines[:n-1]
		trailingNewline = true
	}

	starts := topLevelKeys(lines)
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	type edit struct {
		start, end int
		repl       []string
	}
	var edits []edit
	var appended []string

	for _, k := range keys {
		v := values[k]
		start, exists := starts[k]

		if v == nil {
			if !exists {
				continue
			}
			edits = append(edits, edit{start: start, end: blockEnd(lines, start)})
			continue
		}

		rendered, err := renderKey(k, v)
		if err != nil {
			return nil, err
		}
		if !exists {
			appended = append(appended, rendered...)
			continue
		}
		end := blockEnd(lines, start)
		if sameValue(lines[start:end], rendered) {
			continue // 语义没变，别动文件（也保住用户原来的引号风格）
		}
		edits = append(edits, edit{start: start, end: end, repl: withInnerComments(lines[start:end], rendered)})
	}

	// 从后往前替换，前面的行号才不会失效。
	sort.Slice(edits, func(i, j int) bool { return edits[i].start > edits[j].start })
	for _, e := range edits {
		out := make([]string, 0, len(lines)-(e.end-e.start)+len(e.repl))
		out = append(out, lines[:e.start]...)
		out = append(out, e.repl...)
		out = append(out, lines[e.end:]...)
		lines = out
	}
	lines = append(lines, appended...)

	if trailingNewline || len(appended) > 0 {
		lines = append(lines, "")
	}
	return []byte(strings.Join(lines, eol)), nil
}

// PatchChanged 报告 values 中哪些键会让 PatchYAML 真正改动文件。
// PatchFile 用它算 PatchResult.Changed，避免把"提交了但没变"的键算进去。
func PatchChanged(src []byte, values map[string]any) ([]string, error) {
	patched, err := PatchYAML(src, values)
	if err != nil {
		return nil, err
	}
	if bytes.Equal(src, patched) {
		return nil, nil
	}
	// 逐键判断，得到"改动了哪些键"而不是"整体有没有变"。
	changed := make([]string, 0, len(values))
	for k := range values {
		one, err := PatchYAML(src, map[string]any{k: values[k]})
		if err != nil {
			return nil, err
		}
		if !bytes.Equal(src, one) {
			changed = append(changed, k)
		}
	}
	sort.Strings(changed)
	return changed, nil
}

// checkMapping 确认 src 是合法 YAML 且根节点是映射。
func checkMapping(src []byte) error {
	var doc yaml.Node
	if err := yaml.Unmarshal(src, &doc); err != nil {
		return fmt.Errorf("解析配置失败: %w", err)
	}
	if len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return ErrNotMapping
	}
	return nil
}

// topLevelKeys 返回每个顶层键首次出现的行号。顶层键在行首、后跟冒号；
// 缩进的行、注释行、文档标记都不算。
func topLevelKeys(lines []string) map[string]int {
	keys := make(map[string]int, len(lines))
	for i, line := range lines {
		if line == "" || line[0] == ' ' || line[0] == '\t' || line[0] == '#' || line[0] == '-' {
			continue
		}
		colon := strings.IndexByte(line, ':')
		if colon <= 0 {
			continue
		}
		key := line[:colon]
		if _, dup := keys[key]; dup {
			continue
		}
		keys[key] = i
	}
	return keys
}

// blockEnd 返回从 start 开始的键块结束行号（不含）：键行加上它后面所有缩进的行。
// 空行与顶格注释行终止块，于是它们留在原位不动。
func blockEnd(lines []string, start int) int {
	end := start + 1
	for end < len(lines) {
		if !strings.HasPrefix(lines[end], " ") && !strings.HasPrefix(lines[end], "\t") {
			break
		}
		end++
	}
	return end
}

// renderKey 用 YAML 编码器渲染单个键值，借它处理转义与引号，再按 2 空格缩进
// 拼成行。这样列表值也保持项目惯用的 "  - item" 写法。
func renderKey(key string, v any) ([]string, error) {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(map[string]any{key: v}); err != nil {
		return nil, fmt.Errorf("编码配置项 %s 失败: %w", key, err)
	}
	if err := enc.Close(); err != nil {
		return nil, fmt.Errorf("编码配置项 %s 失败: %w", key, err)
	}
	rendered := strings.Split(strings.TrimSuffix(buf.String(), "\n"), "\n")
	if len(rendered) == 0 || strings.TrimSpace(rendered[0]) == "" {
		return nil, fmt.Errorf("编码配置项 %s 得到空结果", key)
	}
	return rendered, nil
}

// withInnerComments 把旧块里的缩进注释行搬到新块里，插在键行与值之间。
// 这些注释往往是写给用户看的示例（例如 sub-urls 下面那批注释掉的写法），
// 属于文档，不能在改写值的时候当垃圾扔掉。
func withInnerComments(oldBlock, rendered []string) []string {
	var comments []string
	for _, line := range oldBlock[min(1, len(oldBlock)):] {
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			comments = append(comments, line)
		}
	}
	if len(comments) == 0 {
		return rendered
	}
	out := make([]string, 0, len(rendered)+len(comments))
	out = append(out, rendered[0])
	out = append(out, comments...)
	out = append(out, rendered[1:]...)
	return out
}

// sameValue 比较两个键块解析后的值，避开格式差异：
// `:8199` 与 `":8199"` 是同一个值，50 与 50.0 也是。
// 解析不了就当作不同，宁可重写也不要误判成"没改"。
func sameValue(oldBlock, newBlock []string) bool {
	oldVal, ok := blockValue(oldBlock)
	if !ok {
		return false
	}
	newVal, ok := blockValue(newBlock)
	if !ok {
		return false
	}
	return reflect.DeepEqual(normalizeValue(oldVal), normalizeValue(newVal))
}

// blockValue 解析一个键块，返回它唯一那个键的值。
func blockValue(block []string) (any, bool) {
	var parsed map[string]any
	if err := yaml.Unmarshal([]byte(strings.Join(block, "\n")), &parsed); err != nil {
		return nil, false
	}
	if len(parsed) != 1 {
		return nil, false
	}
	for _, v := range parsed {
		return v, true
	}
	return nil, false
}

// normalizeValue 把 YAML 与 JSON 两边的数字、切片形状抹平，好比较。
func normalizeValue(v any) any {
	switch t := v.(type) {
	case int:
		return float64(t)
	case int64:
		return float64(t)
	case uint64:
		return float64(t)
	case []string:
		out := make([]any, 0, len(t))
		for _, e := range t {
			out = append(out, e)
		}
		return out
	case []any:
		out := make([]any, 0, len(t))
		for _, e := range t {
			out = append(out, normalizeValue(e))
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, e := range t {
			out[k] = normalizeValue(e)
		}
		return out
	default:
		return v
	}
}
