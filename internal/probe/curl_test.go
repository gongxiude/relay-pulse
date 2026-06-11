package probe

import (
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// newReq 构造一个带 canonical headers 的请求用于 curl 序列化测试。
func newReq(t *testing.T, method, rawURL string, headers map[string]string) *http.Request {
	t.Helper()
	u, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	req := &http.Request{Method: method, URL: u, Header: http.Header{}}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	return req
}

func TestBuildCurl_RedactsKeyEverywhere(t *testing.T) {
	const key = "sk-secret-1234567890"
	req := newReq(t, "POST", "https://api.example.com/v1/messages?token="+key, map[string]string{
		"Authorization": "Bearer " + key,
		"Content-Type":  "application/json",
	})
	body := []byte(`{"api_key":"` + key + `","model":"x"}`)

	curl := buildCurlCommand(req, body, key)

	if strings.Contains(curl, key) {
		t.Fatalf("curl 不应含明文密钥:\n%s", curl)
	}
	if !strings.Contains(curl, curlAPIKeyEnvExpr) {
		t.Fatalf("curl 应含 %s 变量展开:\n%s", curlAPIKeyEnvExpr, curl)
	}
	// header / url / body 三处命中都应被替换，env 提示行应出现
	if !strings.HasPrefix(curl, "# 复现前设置真实密钥") {
		t.Fatalf("命中密钥应输出 env 提示行:\n%s", curl)
	}
	if strings.Count(curl, curlAPIKeyEnvExpr) != 3 {
		t.Fatalf("期望 url/header/body 三处各替换一次，got %d 次:\n%s", strings.Count(curl, curlAPIKeyEnvExpr), curl)
	}
}

func TestBuildCurl_AuthHeaderSplice(t *testing.T) {
	const key = "tok_abcdefghijklmnop"
	req := newReq(t, "POST", "https://x.test/v1", map[string]string{
		"Authorization": "Bearer " + key,
	})
	curl := buildCurlCommand(req, nil, key)

	// 期望片段：单引号字面量 + 双引号变量拼接，shell 拼成 "Bearer <key>"
	want := `-H 'Authorization: Bearer '"$RP_API_KEY"`
	if !strings.Contains(curl, want) {
		t.Fatalf("期望 header 拼接 %q，实际:\n%s", want, curl)
	}
}

func TestBuildCurl_NoKey(t *testing.T) {
	req := newReq(t, "GET", "https://x.test/v1/models", map[string]string{
		"Accept": "application/json",
	})
	curl := buildCurlCommand(req, nil, "")

	if strings.Contains(curl, "RP_API_KEY") {
		t.Fatalf("无密钥时不应出现变量/提示:\n%s", curl)
	}
	if !strings.Contains(curl, "curl -i --http1.1 -X 'GET'") {
		t.Fatalf("基础命令格式不对:\n%s", curl)
	}
	if !strings.Contains(curl, "'https://x.test/v1/models'") {
		t.Fatalf("URL 应被单引号包裹:\n%s", curl)
	}
}

func TestBuildCurl_ShortKeyNotRedacted(t *testing.T) {
	// 过短的 "key" 不脱敏，避免误命中正常文本子串
	const shortKey = "key"
	req := newReq(t, "POST", "https://x.test/api_key/list", map[string]string{
		"X-Token": "key",
	})
	curl := buildCurlCommand(req, []byte(`{"key":1}`), shortKey)

	if strings.Contains(curl, "RP_API_KEY") {
		t.Fatalf("短 key 不应触发脱敏（否则会破坏 url/body 中的正常文本）:\n%s", curl)
	}
}

func TestBuildCurl_BodyWithSingleQuoteEscaped(t *testing.T) {
	req := newReq(t, "POST", "https://x.test/v1", map[string]string{
		"Content-Type": "application/json",
	})
	body := []byte(`{"prompt":"it's a test"}`)
	curl := buildCurlCommand(req, body, "")

	// 单引号必须转义成 '\'' ；命令仍可被 shell 解析
	if !strings.Contains(curl, `it'\''s a test`) {
		t.Fatalf("body 内单引号应转义成 '\\'':\n%s", curl)
	}
}

func TestBuildCurl_HeadersSortedStable(t *testing.T) {
	req := newReq(t, "POST", "https://x.test/v1", map[string]string{
		"Z-Header": "1",
		"A-Header": "2",
		"M-Header": "3",
	})
	curl := buildCurlCommand(req, nil, "")

	ai := strings.Index(curl, "A-Header")
	mi := strings.Index(curl, "M-Header")
	zi := strings.Index(curl, "Z-Header")
	if !(ai < mi && mi < zi) {
		t.Fatalf("header 应按字典序输出，got A=%d M=%d Z=%d:\n%s", ai, mi, zi, curl)
	}
}

func TestBuildCurl_RedactsURLEncodedKeyInPath(t *testing.T) {
	// key 含会被 req.URL.String() 重编码的字符，且落在 path 段：原始 key 匹配不到，
	// 必须靠 PathEscape 变体脱敏，否则 %xx 形态的密钥会泄漏。
	const key = "sk secret/with+special$chars"
	encoded := url.PathEscape(key)
	req := newReq(t, "GET", "https://api.example.com/keys/"+encoded+"/models", nil)

	curl := buildCurlCommand(req, nil, key)

	if strings.Contains(curl, encoded) {
		t.Fatalf("curl 不应含 URL 编码后的明文密钥 %q:\n%s", encoded, curl)
	}
	if !strings.Contains(curl, curlAPIKeyEnvExpr) {
		t.Fatalf("path 中的密钥应被脱敏成变量:\n%s", curl)
	}
}

func TestRedactSecrets_ErrorMessage(t *testing.T) {
	// key 含会被 QueryEscape 编码的字符（'/' -> %2F），模拟 *url.Error 同时带
	// query 编码形态与 header 原始形态两种泄漏面。
	const key = "sk-ant/oat01+abcdef1234567890"
	encoded := url.QueryEscape(key)
	if encoded == key {
		t.Fatalf("测试前提失效：key 应被 QueryEscape 改写")
	}
	msg := `Get "https://x.test/v1?token=` + encoded + `": dial tcp lookup failed (sent ` + key + `)`

	got := redactSecrets(msg, secretVariants(key))
	if strings.Contains(got, key) || strings.Contains(got, encoded) {
		t.Fatalf("错误文案仍含明文/编码密钥:\n%s", got)
	}
	if !strings.Contains(got, redactedKeyMarker) {
		t.Fatalf("应替换为占位符 %q:\n%s", redactedKeyMarker, got)
	}
}

func TestRedactSecrets_ShortKeyNoop(t *testing.T) {
	// 短 key 不脱敏，避免误伤正常文案
	msg := "connection refused for key"
	if got := redactSecrets(msg, secretVariants("key")); got != msg {
		t.Fatalf("短 key 不应改动文案，got %q", got)
	}
}

func TestBuildCurl_NilRequest(t *testing.T) {
	if got := buildCurlCommand(nil, nil, "k"); got != "" {
		t.Fatalf("nil 请求应返回空串，got %q", got)
	}
}

// TestBuildCurl_ValidShellSyntax 端到端验证：生成的 curl 必须是语法合法的 shell，
// 且 $RP_API_KEY 在拼接处能正确展开成真实密钥——脱敏的单/双引号交替拼接最易在此翻车。
func TestBuildCurl_ValidShellSyntax(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash 不可用，跳过 shell 语法校验")
	}

	const key = "sk-ant-oat01-AbC'dEf$gHi&jKl_1234567890"
	req := newReq(t, "POST", "https://gw.example.com/v1/messages?beta=true&token="+url.QueryEscape(key), map[string]string{
		"Authorization": "Bearer " + key,
		"Content-Type":  "application/json",
		"X-Api-Key":     key,
	})
	body := []byte(`{"model":"x","messages":[{"role":"user","content":"it's 1+1 & don't break $PATH"}]}`)

	curl := buildCurlCommand(req, body, key)
	if strings.Contains(curl, "dEf$gHi") {
		t.Fatalf("curl 仍含明文密钥片段:\n%s", curl)
	}

	// 1) bash -n 仅做语法解析，不执行（curl 本身可不存在）
	script := curl + "\n"
	tmp, err := os.CreateTemp(t.TempDir(), "curl-*.sh")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tmp.WriteString(script); err != nil {
		t.Fatal(err)
	}
	tmp.Close()
	if out, err := exec.Command(bash, "-n", tmp.Name()).CombinedOutput(); err != nil {
		t.Fatalf("生成的 curl 不是合法 shell:\n%s\n--- bash ---\n%s", curl, out)
	}

	// 2) 把 curl 换成 printf 验证 $RP_API_KEY 真能展开成真实密钥（注入到拼接点）
	//    用 declare -f 不便，这里直接把命令里的 'curl ' 替成 'printf %s\n ' 并设变量执行。
	probeScript := "export RP_API_KEY='" + strings.ReplaceAll(key, "'", `'\''`) + "'\n" +
		strings.Replace(curl, "curl -i --http1.1", "printf '%s\\0'", 1)
	out, err := exec.Command(bash, "-c", probeScript).CombinedOutput()
	if err != nil {
		t.Fatalf("注入真实密钥执行失败:\n%s\n--- err ---\n%v\n%s", probeScript, err, out)
	}
	if !strings.Contains(string(out), key) {
		t.Fatalf("$RP_API_KEY 未展开成真实密钥；输出片段缺失。output=%q", out)
	}
}
