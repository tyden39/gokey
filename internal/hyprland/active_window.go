package hyprland

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"time"
)

var ErrNoActiveWindow = errors.New("Hyprland returned no active window")

type ActiveWindow struct {
	Address      string `json:"address"`
	Class        string `json:"class"`
	InitialClass string `json:"initialClass"`
	PID          int    `json:"pid"`
	XWayland     bool   `json:"xwayland"`
}

func (w ActiveWindow) Identity() string {
	if w.InitialClass != "" {
		return w.InitialClass
	}
	return w.Class
}

func ActiveWindowFromEnvironment(timeout time.Duration) (ActiveWindow, error) {
	runtimeDir := os.Getenv("XDG_RUNTIME_DIR")
	if runtimeDir == "" {
		return ActiveWindow{}, errors.New("XDG_RUNTIME_DIR is empty")
	}
	signature := os.Getenv("HYPRLAND_INSTANCE_SIGNATURE")
	if signature == "" {
		return ActiveWindow{}, errors.New("HYPRLAND_INSTANCE_SIGNATURE is empty")
	}
	if filepath.Base(signature) != signature {
		return ActiveWindow{}, errors.New("invalid HYPRLAND_INSTANCE_SIGNATURE")
	}
	socketPath := filepath.Join(runtimeDir, "hypr", signature, ".socket.sock")
	return QueryActiveWindow(socketPath, timeout)
}

func QueryActiveWindow(socketPath string, timeout time.Duration) (ActiveWindow, error) {
	conn, err := net.DialTimeout("unix", socketPath, timeout)
	if err != nil {
		return ActiveWindow{}, fmt.Errorf("connect Hyprland IPC: %w", err)
	}
	defer conn.Close()

	if err := conn.SetDeadline(time.Now().Add(timeout)); err != nil {
		return ActiveWindow{}, fmt.Errorf("set Hyprland IPC deadline: %w", err)
	}
	if _, err := io.WriteString(conn, "j/activewindow"); err != nil {
		return ActiveWindow{}, fmt.Errorf("request active window: %w", err)
	}

	var window ActiveWindow
	if err := json.NewDecoder(conn).Decode(&window); err != nil {
		return ActiveWindow{}, fmt.Errorf("decode active window: %w", err)
	}
	if window.Address == "" || window.Identity() == "" {
		return ActiveWindow{}, ErrNoActiveWindow
	}
	return window, nil
}
