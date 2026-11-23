package hyprland

import (
	"io"
	"net"
	"path/filepath"
	"testing"
	"time"
)

func TestQueryActiveWindow(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hypr.sock")
	listener, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	done := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			done <- err
			return
		}
		defer conn.Close()
		request := make([]byte, len("j/activewindow"))
		if _, err := io.ReadFull(conn, request); err != nil {
			done <- err
			return
		}
		if string(request) != "j/activewindow" {
			done <- &unexpectedRequest{got: string(request)}
			return
		}
		_, err = io.WriteString(conn, `{"address":"0xabc","class":"Chromium","initialClass":"chromium","pid":123,"xwayland":false}`)
		done <- err
	}()

	window, err := QueryActiveWindow(path, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if window.Identity() != "chromium" || window.Address != "0xabc" || window.PID != 123 || window.XWayland {
		t.Fatalf("unexpected window: %+v", window)
	}
}

func TestIdentityFallsBackToClass(t *testing.T) {
	window := ActiveWindow{Class: "fallback"}
	if got := window.Identity(); got != "fallback" {
		t.Fatalf("identity = %q", got)
	}
}

func TestQueryActiveWindowMissingSocket(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.sock")
	if _, err := QueryActiveWindow(path, time.Millisecond); err == nil {
		t.Fatal("missing socket should fail")
	}
}

func TestQueryActiveWindowRejectsEmptyResponse(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hypr.sock")
	listener, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		request := make([]byte, len("j/activewindow"))
		_, _ = io.ReadFull(conn, request)
		_, _ = io.WriteString(conn, `{}`)
	}()

	if _, err := QueryActiveWindow(path, time.Second); err != ErrNoActiveWindow {
		t.Fatalf("empty response error = %v", err)
	}
}

func TestActiveWindowFromEnvironmentRejectsInvalidSignature(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	t.Setenv("HYPRLAND_INSTANCE_SIGNATURE", "../other")
	if _, err := ActiveWindowFromEnvironment(time.Millisecond); err == nil {
		t.Fatal("invalid signature should fail")
	}
}

type unexpectedRequest struct {
	got string
}

func (e *unexpectedRequest) Error() string {
	return "unexpected request: " + e.got
}
