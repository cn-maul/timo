//go:build linux

package app

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
)

// isWayland returns true when the desktop session runs on a Wayland compositor.
func isWayland() bool {
	if os.Getenv("WAYLAND_DISPLAY") != "" {
		return true
	}
	sessionType := strings.ToLower(os.Getenv("XDG_SESSION_TYPE"))
	return strings.Contains(sessionType, "wayland")
}

// ciPattern wraps a string in a POSIX-compatible case-insensitive character class.
// Example: "Claude" → "[Cc][Ll][Aa][Uu][Dd][Ee]"
func ciPattern(s string) string {
	var b strings.Builder
	for _, r := range s {
		lo := strings.ToLower(string(r))
		up := strings.ToUpper(string(r))
		if lo == up {
			b.WriteString(lo)
		} else {
			fmt.Fprintf(&b, "[%s%s]", lo, up)
		}
	}
	return b.String()
}

// focusAITerminal attempts to bring the AI tool terminal window to the foreground.
// On Wayland it tries compositor-specific tools; on X11 it uses xdotool.
func focusAITerminal(response string) {
	_ = response

	if isWayland() {
		if focusWayland() {
			return
		}
		log.Printf("timo: wayland detected but could not activate window; install ydotool or a compositor-specific tool (swaymsg/hyprctl)")
		return
	}

	// X11: use xdotool directly (no sh -c to avoid shell injection)
	for _, pattern := range x11WindowPatterns() {
		if activateXdotool(pattern) {
			return
		}
	}
	// Fallback: try common terminal names
	for _, pattern := range x11TerminalPatterns() {
		if activateXdotool(pattern) {
			return
		}
	}
}

// ---------------------------------------------------------------------------
// X11 helpers
// ---------------------------------------------------------------------------

type x11Pattern struct{ flag, value string }

func x11WindowPatterns() []x11Pattern {
	return []x11Pattern{
		{"--class", "claude"},
		{"--classname", "claude"},
		{"--name", "Claude"},
		{"--class", "reasonix"},
		{"--classname", "reasonix"},
		{"--name", "Reasonix"},
	}
}

func x11TerminalPatterns() []x11Pattern {
	return []x11Pattern{
		{"--name", "Terminal"},
		{"--name", "terminal"},
		{"--class", "terminal"},
		{"--class", "kitty"},
		{"--class", "alacritty"},
		{"--class", "wezterm"},
	}
}

// activateXdotool searches for a window and activates (focuses) it.
// --limit 1 ensures only the first match is activated.
func activateXdotool(p x11Pattern) bool {
	// xdotool search [flags] key value windowactivate
	out, err := exec.Command("xdotool", "search", "--limit", "1",
		p.flag, p.value, "windowactivate").Output()
	return err == nil && len(out) > 0
}

// ---------------------------------------------------------------------------
// Wayland compositor support
// ---------------------------------------------------------------------------

// focusWayland tries each available strategy in turn and returns true on success.
func focusWayland() bool {
	if runSway() {
		return true
	}
	if runHyprland() {
		return true
	}
	if runKDE() {
		return true
	}
	if runGnome() {
		return true
	}
	if runYdotool() {
		return true
	}
	return false
}

// --- Sway / i3 (wlroots) ---

func runSway() bool {
	if os.Getenv("SWAYSOCK") == "" && os.Getenv("I3SOCK") == "" {
		return false
	}
	if !toolInstalled("swaymsg") {
		return false
	}
	for _, criteria := range []string{
		`[app_id="(?i)claude"] focus`,
		`[title="(?i)claude"] focus`,
		`[app_id="(?i)reasonix"] focus`,
		`[title="(?i)reasonix"] focus`,
	} {
		// swaymsg sends the entire IPC message as a single argument.
		if tryActivate("swaymsg", criteria) {
			return true
		}
	}
	return false
}

// --- Hyprland ---

func runHyprland() bool {
	if os.Getenv("HYPRLAND_INSTANCE_SIGNATURE") == "" {
		return false
	}
	if !toolInstalled("hyprctl") {
		return false
	}
	for _, name := range []string{"Claude", "claude", "Reasonix", "reasonix"} {
		// Use POSIX-compatible character-class pattern for case-insensitive matching.
		pat := ciPattern(name)
		if tryActivate("hyprctl", "dispatch", "focuswindow", "title:"+pat) {
			return true
		}
		if tryActivate("hyprctl", "dispatch", "focuswindow", "class:"+pat) {
			return true
		}
	}
	return false
}

// --- KDE / kwin ---

func runKDE() bool {
	if !isKDE() {
		return false
	}
	if !toolInstalled("qdbus") {
		return false
	}
	for _, needle := range []string{"claude", "reasonix", "terminal", "Terminal"} {
		// The script returns true + breaks out immediately when a match is found.
		// The trailing `false` is the fallback return value.
		script := fmt.Sprintf(
			`var ws=workspace; for(var i=0;i<ws.clientList().length;i++){var c=ws.clientList()[i];if(c.caption.toLowerCase().indexOf('%s')>=0){ws.activateClient(c);return true;}}false`,
			needle,
		)
		cmd := exec.Command("qdbus", "org.kde.KWin", "/Scripting", "runScript", script)
		out, err := cmd.CombinedOutput()
		if err != nil {
			continue
		}
		if strings.TrimSpace(string(out)) == "true" {
			return true
		}
	}
	return false
}

// --- GNOME Shell ---

func runGnome() bool {
	if !isGnomeShell() {
		return false
	}
	if !toolInstalled("gdbus") {
		return false
	}
	for _, needle := range []string{"claude", "reasonix"} {
		// Return the number of matched windows as the eval result.
		script := fmt.Sprintf(
			`const w=global.get_window_actors().filter(a=>a.meta_window.get_title().toLowerCase().indexOf('%s')>=0);w.forEach(a=>a.meta_window.activate(global.get_current_time()));w.length`,
			needle,
		)
		cmd := exec.Command("gdbus", "call", "--session",
			"--dest", "org.gnome.Shell",
			"--object-path", "/org/gnome/Shell",
			"--method", "org.gnome.Shell.Eval",
			script,
		)
		out, _ := cmd.Output()
		// Output format: (true, 'N') where N is the count of windows found.
		// Return true only when eval succeeded AND at least one window matched.
		result := strings.TrimSpace(string(out))
		if strings.HasPrefix(result, "(true") && !strings.Contains(result, ", '0'") {
			return true
		}
	}
	return false
}

// --- Generic ydotool (works on any Wayland compositor) ---

func runYdotool() bool {
	if !toolInstalled("ydotool") {
		return false
	}
	// Simulate pressing and releasing the Super (Meta) key — opens the
	// launcher / overview on most Wayland desktops.  This is a best-effort
	// fallback and may not work on all compositors.
	return exec.Command("ydotool", "key", "125:1", "125:0").Run() == nil
	// 125 = KEY_LEFTMETA (from linux/input-event-codes.h)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// isKDE returns true when running on KDE / kwin.
func isKDE() bool {
	return os.Getenv("KDE_FULL_SESSION") == "true" ||
		strings.Contains(strings.ToLower(os.Getenv("DESKTOP_SESSION")), "plasma") ||
		strings.Contains(os.Getenv("XDG_CURRENT_DESKTOP"), "KDE")
}

// isGnomeShell returns true when running on GNOME Shell or a variant that
// exposes the org.gnome.Shell.Eval D-Bus method.
func isGnomeShell() bool {
	desktop := os.Getenv("XDG_CURRENT_DESKTOP")
	return desktop == "GNOME" ||
		desktop == "Unity" ||
		desktop == "ubuntu:GNOME" ||
		desktop == "Budgie:GNOME" ||
		desktop == "Pantheon"
}

// toolInstalled returns true when the named executable is on $PATH.
func toolInstalled(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// tryActivate runs args as a command and returns true when it exits 0 with
// non-empty output.
func tryActivate(args ...string) bool {
	if len(args) == 0 || args[0] == "" {
		return false
	}
	cmd := exec.Command(args[0], args[1:]...)
	out, err := cmd.Output()
	return err == nil && len(out) > 0
}
