package probe

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"testing"
)

// timeoutErr 实现 net.Error 且 Timeout()=true，模拟传输层读超时。
type timeoutErr struct{}

func (timeoutErr) Error() string   { return "i/o timeout" }
func (timeoutErr) Timeout() bool   { return true }
func (timeoutErr) Temporary() bool { return false }

var _ net.Error = timeoutErr{}

func TestBodyReadSubStatus(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"oversize sentinel", fmt.Errorf("%w（10 bytes）", errResponseTooLarge), "response_too_large"},
		{"context deadline", context.DeadlineExceeded, "response_timeout"},
		{"wrapped context deadline", fmt.Errorf("read body: %w", context.DeadlineExceeded), "response_timeout"},
		{"os deadline", os.ErrDeadlineExceeded, "response_timeout"},
		{"net timeout", timeoutErr{}, "response_timeout"},
		{"connection reset", errors.New("connection reset by peer"), "network_error"},
		{"unexpected EOF", io.ErrUnexpectedEOF, "network_error"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := bodyReadSubStatus(c.err); got != c.want {
				t.Errorf("bodyReadSubStatus(%v) = %q, want %q", c.err, got, c.want)
			}
		})
	}
}

// 确认 readBodyLimited 的超大错误带上哨兵，能被 errors.Is 识别。
func TestReadBodyLimitedOversizeWrapsSentinel(t *testing.T) {
	_, err := readBodyLimited(io.LimitReader(neverEOF{}, 100), 4)
	if err == nil {
		t.Fatal("超过上限应返回错误")
	}
	if !errors.Is(err, errResponseTooLarge) {
		t.Errorf("超大错误应包裹 errResponseTooLarge，实际: %v", err)
	}
	if bodyReadSubStatus(err) != "response_too_large" {
		t.Errorf("超大错误应归类 response_too_large，实际 %q", bodyReadSubStatus(err))
	}
}

// neverEOF 持续吐字节，触发超过上限分支。
type neverEOF struct{}

func (neverEOF) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 'x'
	}
	return len(p), nil
}
