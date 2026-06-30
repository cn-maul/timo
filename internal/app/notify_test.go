//go:build linux

package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Helper: write a JSON settings file at the given path, creating parent dirs.
func writeJSON(t *testing.T, path string, v interface{}) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent: %v", err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

// Helper: read and parse a JSON settings file.
func readJSON(t *testing.T, path string) map[string]interface{} {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	return m
}

// ── buildHooksConfig tests ──

func TestBuildHooksConfig_Claude_HasRequiredEvents(t *testing.T) {
	const timoPath = "/usr/bin/timo"
	cfg := buildHooksConfig(timoPath, "claude")

	requiredEvents := []string{
		"UserPromptSubmit", "PreToolUse", "PostToolUse",
		"SubagentStart", "SubagentStop", "Stop", "Notification",
	}
	for _, event := range requiredEvents {
		if _, ok := cfg[event]; !ok {
			t.Errorf("claude config missing event: %s", event)
		}
	}
}

func TestBuildHooksConfig_Claude_ContainsTimoPath(t *testing.T) {
	const timoPath = "/opt/timo/timo"
	cfg := buildHooksConfig(timoPath, "claude")

	for event, raw := range cfg {
		entries, ok := raw.([]interface{})
		if !ok {
			t.Fatalf("event %s: expected []interface{}, got %T", event, raw)
		}
		for i, entry := range entries {
			m, ok := entry.(map[string]interface{})
			if !ok {
				t.Fatalf("event %s[%d]: expected map, got %T", event, i, entry)
			}
			hooksArr, ok := m["hooks"].([]interface{})
			if !ok {
				t.Fatalf("event %s[%d]: missing hooks array", event, i)
			}
			for j, h := range hooksArr {
				hookMap, ok := h.(map[string]interface{})
				if !ok {
					t.Fatalf("event %s[%d][%d]: expected map", event, i, j)
				}
				cmd, ok := hookMap["command"].(string)
				if !ok {
					t.Fatalf("event %s[%d][%d]: missing command", event, i, j)
				}
				if !strings.HasPrefix(cmd, timoPath) {
					t.Errorf("event %s[%d][%d]: command %q does not start with %s", event, i, j, cmd, timoPath)
				}
				if !strings.Contains(cmd, " notify --type claude-") {
					t.Errorf("event %s[%d][%d]: command %q missing 'notify --type claude-'", event, i, j, cmd)
				}
			}
		}
	}
}

func TestBuildHooksConfig_Claude_NestedFormat(t *testing.T) {
	cfg := buildHooksConfig("/usr/bin/timo", "claude")

	for event, raw := range cfg {
		entries, ok := raw.([]interface{})
		if !ok {
			t.Errorf("event %s: not []interface{}", event)
			continue
		}
		for i, entry := range entries {
			m, ok := entry.(map[string]interface{})
			if !ok {
				t.Errorf("event %s[%d]: not map", event, i)
				continue
			}
			if _, ok := m["hooks"]; !ok {
				t.Errorf("event %s[%d]: missing 'hooks' key (Claude format requires nested hooks)", event, i)
			}
		}
	}
}

func TestBuildHooksConfig_Reasonix_HasRequiredEvents(t *testing.T) {
	const timoPath = "/usr/bin/timo"
	cfg := buildHooksConfig(timoPath, "reasonix")

	requiredEvents := []string{
		"SessionStart", "UserPromptSubmit", "PreToolUse", "PostToolUse",
		"PostLLMCall", "SubagentStop", "Stop", "Notification", "PreCompact",
	}
	for _, event := range requiredEvents {
		if _, ok := cfg[event]; !ok {
			t.Errorf("reasonix config missing event: %s", event)
		}
	}
}

func TestBuildHooksConfig_Reasonix_FlatFormat(t *testing.T) {
	cfg := buildHooksConfig("/usr/bin/timo", "reasonix")

	for event, raw := range cfg {
		entries, ok := raw.([]interface{})
		if !ok {
			t.Errorf("event %s: not []interface{}", event)
			continue
		}
		for i, entry := range entries {
			m, ok := entry.(map[string]interface{})
			if !ok {
				t.Errorf("event %s[%d]: not map", event, i)
				continue
			}
			if _, ok := m["hooks"]; ok {
				t.Errorf("event %s[%d]: has 'hooks' key (should be flat for reasonix)", event, i)
			}
			if _, ok := m["command"]; !ok {
				t.Errorf("event %s[%d]: missing 'command' key (reasonix flat format)", event, i)
			}
		}
	}
}

func TestBuildHooksConfig_Reasonix_ContainsTimoPath(t *testing.T) {
	const timoPath = "/opt/reasonix/timo"
	cfg := buildHooksConfig(timoPath, "reasonix")

	for event, raw := range cfg {
		entries, ok := raw.([]interface{})
		if !ok {
			continue
		}
		for i, entry := range entries {
			m, ok := entry.(map[string]interface{})
			if !ok {
				continue
			}
			cmd, ok := m["command"].(string)
			if !ok {
				t.Errorf("event %s[%d]: missing command", event, i)
				continue
			}
			if !strings.HasPrefix(cmd, timoPath) {
				t.Errorf("event %s[%d]: command %q does not start with %s", event, i, cmd, timoPath)
			}
		}
	}
}

func TestBuildHooksConfig_Reasonix_AllCommandsContainNotify(t *testing.T) {
	cfg := buildHooksConfig("/usr/bin/timo", "reasonix")

	for event, raw := range cfg {
		entries, ok := raw.([]interface{})
		if !ok {
			continue
		}
		for i, entry := range entries {
			m, ok := entry.(map[string]interface{})
			if !ok {
				continue
			}
			cmd, _ := m["command"].(string)
			if !strings.Contains(cmd, "timo notify") {
				t.Errorf("event %s[%d]: command %q does not contain 'timo notify'", event, i, cmd)
			}
		}
	}
}

func TestBuildHooksConfig_Claude_AllCommandsContainNotify(t *testing.T) {
	cfg := buildHooksConfig("/usr/bin/timo", "claude")

	for event, raw := range cfg {
		entries, ok := raw.([]interface{})
		if !ok {
			continue
		}
		for i, entry := range entries {
			m, ok := entry.(map[string]interface{})
			if !ok {
				continue
			}
			hooksArr, ok := m["hooks"].([]interface{})
			if !ok {
				continue
			}
			for j, h := range hooksArr {
				hookMap, ok := h.(map[string]interface{})
				if !ok {
					continue
				}
				cmd, _ := hookMap["command"].(string)
				if !strings.Contains(cmd, "timo notify") {
					t.Errorf("event %s[%d][%d]: command %q does not contain 'timo notify'", event, i, j, cmd)
				}
			}
		}
	}
}

// ── isTimoHook tests ──

func TestIsTimoHook_True(t *testing.T) {
	entry := map[string]interface{}{"command": "/usr/bin/timo notify --type claude-prompt"}
	if !isTimoHook(entry) {
		t.Error("expected isTimoHook to return true for timo notify command")
	}
}

func TestIsTimoHook_False(t *testing.T) {
	entry := map[string]interface{}{"command": "/usr/bin/other-tool --do-something"}
	if isTimoHook(entry) {
		t.Error("expected isTimoHook to return false for non-timo command")
	}
}

func TestIsTimoHook_EmptyCommand(t *testing.T) {
	entry := map[string]interface{}{"command": ""}
	if isTimoHook(entry) {
		t.Error("expected isTimoHook to return false for empty command")
	}
}

func TestIsTimoHook_NoCommand(t *testing.T) {
	entry := map[string]interface{}{"something": "else"}
	if isTimoHook(entry) {
		t.Error("expected isTimoHook to return false when no command key")
	}
}

// ── isTimoHookNested tests ──

func TestIsTimoHookNested_DirectCommand(t *testing.T) {
	m := map[string]interface{}{
		"command": "/usr/bin/timo notify --type claude-prompt",
	}
	if !isTimoHookNested(m) {
		t.Error("expected true for direct timo command")
	}
}

func TestIsTimoHookNested_NestedHooks(t *testing.T) {
	m := map[string]interface{}{
		"hooks": []interface{}{
			map[string]interface{}{
				"command": "/usr/bin/timo notify --type claude-stop",
			},
		},
	}
	if !isTimoHookNested(m) {
		t.Error("expected true for nested timo command")
	}
}

func TestIsTimoHookNested_NoTimoCommand(t *testing.T) {
	m := map[string]interface{}{
		"hooks": []interface{}{
			map[string]interface{}{
				"command": "/usr/bin/something-else",
			},
		},
	}
	if isTimoHookNested(m) {
		t.Error("expected false for non-timo nested command")
	}
}

func TestIsTimoHookNested_EmptyMap(t *testing.T) {
	m := map[string]interface{}{}
	if isTimoHookNested(m) {
		t.Error("expected false for empty map")
	}
}

// ── installHooks integration tests ──

// setupTestHome creates a temporary HOME directory and overrides the environment.
func setupTestHome(t *testing.T) string {
	t.Helper()
	origHome := os.Getenv("HOME")
	tmpHome := t.TempDir()
	os.Setenv("HOME", tmpHome)
	t.Cleanup(func() {
		os.Setenv("HOME", origHome)
	})
	return tmpHome
}

func TestInstallHooks_Claude_CreatesSettingsFile(t *testing.T) {
	home := setupTestHome(t)

	timoPath, err := installHooks(false, ".claude", "claude", "test success")
	if err != nil {
		t.Fatalf("installHooks returned error: %v", err)
	}
	if timoPath == "" {
		t.Fatal("installHooks returned empty timoPath")
	}

	settingsPath := filepath.Join(home, ".claude", "settings.json")
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("settings.json not created: %v", err)
	}

	var settings map[string]interface{}
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("settings.json is not valid JSON: %v", err)
	}

	hooks, ok := settings["hooks"].(map[string]interface{})
	if !ok {
		t.Fatal("settings.json missing 'hooks' key")
	}

	requiredEvents := []string{"UserPromptSubmit", "PreToolUse", "PostToolUse", "Stop", "Notification"}
	for _, event := range requiredEvents {
		if _, ok := hooks[event]; !ok {
			t.Errorf("hooks missing event: %s", event)
		}
	}
}

func TestInstallHooks_Reasonix_CreatesSettingsFile(t *testing.T) {
	home := setupTestHome(t)

	timoPath, err := installHooks(false, ".reasonix", "reasonix", "test success")
	if err != nil {
		t.Fatalf("installHooks returned error: %v", err)
	}
	if timoPath == "" {
		t.Fatal("installHooks returned empty timoPath")
	}

	settingsPath := filepath.Join(home, ".reasonix", "settings.json")
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("settings.json not created: %v", err)
	}

	var settings map[string]interface{}
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("settings.json is not valid JSON: %v", err)
	}

	hooks, ok := settings["hooks"].(map[string]interface{})
	if !ok {
		t.Fatal("settings.json missing 'hooks' key")
	}

	requiredEvents := []string{"SessionStart", "UserPromptSubmit", "PreToolUse", "PostToolUse", "Stop", "PreCompact"}
	for _, event := range requiredEvents {
		if _, ok := hooks[event]; !ok {
			t.Errorf("hooks missing event: %s", event)
		}
	}
}

func TestInstallHooks_MergesWithExistingSettings(t *testing.T) {
	home := setupTestHome(t)

	// Pre-create settings.json with existing nested-format hooks (Claude format)
	// and other top-level settings
	existingSettings := map[string]interface{}{
		"permissions": map[string]interface{}{
			"allow": []string{"Bash(npm run *)"},
		},
		"hooks": map[string]interface{}{
			"PreToolUse": []interface{}{
				map[string]interface{}{
					"matcher": "*",
					"hooks": []interface{}{
						map[string]interface{}{
							"type":        "command",
							"command":     "/usr/bin/my-other-hook --pre-tool",
							"description": "other hook",
						},
					},
				},
			},
		},
	}
	settingsPath := filepath.Join(home, ".claude", "settings.json")
	writeJSON(t, settingsPath, existingSettings)

	_, err := installHooks(false, ".claude", "claude", "test success")
	if err != nil {
		t.Fatalf("installHooks returned error: %v", err)
	}

	settings := readJSON(t, settingsPath)

	// Existing permissions should be preserved
	if perms, ok := settings["permissions"].(map[string]interface{}); ok {
		if _, ok := perms["allow"]; !ok {
			t.Error("existing permissions.allow was lost after merge")
		}
	} else {
		t.Error("permissions section lost after merge")
	}

	hooks, ok := settings["hooks"].(map[string]interface{})
	if !ok {
		t.Fatal("hooks section lost after merge")
	}

	// PreToolUse should now contain both the other hook AND the timo hook
	preToolHooks, ok := hooks["PreToolUse"].([]interface{})
	if !ok {
		t.Fatal("PreToolUse hooks lost")
	}

	hasOtherHook := false
	hasTimoHook := false
	for _, entry := range preToolHooks {
		m, ok := entry.(map[string]interface{})
		if !ok {
			continue
		}
		// Check nested hooks array (Claude format)
		hooksArr, ok := m["hooks"].([]interface{})
		if !ok {
			continue
		}
		for _, h := range hooksArr {
			hookMap, ok := h.(map[string]interface{})
			if !ok {
				continue
			}
			cmd, ok := hookMap["command"].(string)
			if !ok {
				continue
			}
			if strings.Contains(cmd, "my-other-hook") {
				hasOtherHook = true
			}
			if strings.Contains(cmd, "notify --type") {
				hasTimoHook = true
			}
		}
	}

	if !hasOtherHook {
		t.Error("other hook was lost during merge")
	}
	if !hasTimoHook {
		t.Error("timo hook was not added during merge")
	}
}

func TestInstallHooks_AutoMode_SkipsIfAlreadyConfigured(t *testing.T) {
	home := setupTestHome(t)

	// Manually create settings with timo hooks so the auto-mode detection works
	// regardless of the test binary name.
	settingsData := map[string]interface{}{
		"hooks": map[string]interface{}{
			"PreToolUse": []interface{}{
				map[string]interface{}{
					"command": "/usr/bin/timo notify --type claude-pre-tool",
				},
			},
		},
	}
	settingsPath := filepath.Join(home, ".claude", "settings.json")
	writeJSON(t, settingsPath, settingsData)

	data1, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}

	// Auto install should detect existing timo hooks and skip
	timoPath, err := installHooks(true, ".claude", "claude", "test success")
	if err != nil {
		t.Fatalf("auto installHooks returned error: %v", err)
	}
	if timoPath != "" {
		t.Errorf("expected empty timoPath when already configured, got %q", timoPath)
	}

	data2, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}

	if string(data1) != string(data2) {
		t.Error("settings.json was modified during auto mode when hooks already existed")
	}
}

func TestInstallHooks_AutoMode_InstallsIfNotConfigured(t *testing.T) {
	setupTestHome(t)

	timoPath, err := installHooks(true, ".claude", "claude", "test success")
	if err != nil {
		t.Fatalf("auto installHooks returned error: %v", err)
	}
	if timoPath == "" {
		t.Error("expected non-empty timoPath when installing fresh in auto mode")
	}
}

func TestInstallHooks_InvalidJSON_ReturnsError(t *testing.T) {
	home := setupTestHome(t)

	settingsPath := filepath.Join(home, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(settingsPath, []byte("{invalid json!!!"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err := installHooks(false, ".claude", "claude", "test success")
	if err == nil {
		t.Error("expected error for invalid JSON, got nil")
	}
}

func TestInstallHooks_PreservesNonHookEvents(t *testing.T) {
	home := setupTestHome(t)

	existingSettings := map[string]interface{}{
		"customKey": "customValue",
		"hooks": map[string]interface{}{
			"CustomEvent": []interface{}{
				map[string]interface{}{
					"command":     "/usr/bin/custom-hook",
					"description": "my custom hook",
				},
			},
		},
	}
	settingsPath := filepath.Join(home, ".claude", "settings.json")
	writeJSON(t, settingsPath, existingSettings)

	_, err := installHooks(false, ".claude", "claude", "test success")
	if err != nil {
		t.Fatalf("installHooks: %v", err)
	}

	settings := readJSON(t, settingsPath)

	if settings["customKey"] != "customValue" {
		t.Error("non-hook custom key was lost during merge")
	}

	hooks := settings["hooks"].(map[string]interface{})
	if _, ok := hooks["CustomEvent"]; !ok {
		t.Error("non-timo CustomEvent was lost during merge")
	}
}

// ── removeHooks tests ──

func TestRemoveHooks_RemovesTimoHooks(t *testing.T) {
	home := setupTestHome(t)

	// Use "timo" (the real binary name) so that isTimoHookNested detection works.
	settingsData := map[string]interface{}{
		"hooks": map[string]interface{}{
			"PreToolUse": []interface{}{
				map[string]interface{}{
					"command": "/usr/bin/timo notify --type claude-pre-tool",
				},
			},
		},
	}
	settingsPath := filepath.Join(home, ".claude", "settings.json")
	writeJSON(t, settingsPath, settingsData)

	if err := removeHooks(".claude"); err != nil {
		t.Fatalf("removeHooks: %v", err)
	}

	settings := readJSON(t, settingsPath)
	if _, ok := settings["hooks"]; ok {
		t.Error("hooks section still exists after removeHooks")
	}
}

func TestRemoveHooks_PreservesOtherSettings(t *testing.T) {
	home := setupTestHome(t)

	settingsData := map[string]interface{}{
		"permissions": map[string]interface{}{
			"allow": []string{"Bash(npm run *)"},
		},
		"hooks": map[string]interface{}{
			"PreToolUse": []interface{}{
				map[string]interface{}{
					"command": "/usr/bin/timo notify --type claude-pre-tool",
				},
			},
		},
	}
	settingsPath := filepath.Join(home, ".claude", "settings.json")
	writeJSON(t, settingsPath, settingsData)

	if err := removeHooks(".claude"); err != nil {
		t.Fatalf("removeHooks: %v", err)
	}

	settings := readJSON(t, settingsPath)
	if perms, ok := settings["permissions"].(map[string]interface{}); ok {
		if _, ok := perms["allow"]; !ok {
			t.Error("permissions.allow lost after removeHooks")
		}
	} else {
		t.Error("permissions section lost after removeHooks")
	}
}

func TestRemoveHooks_NoopWhenNoFile(t *testing.T) {
	setupTestHome(t)

	if err := removeHooks(".claude"); err != nil {
		t.Fatalf("removeHooks on non-existent file: %v", err)
	}
}

func TestRemoveHooks_NoopWhenNoHooks(t *testing.T) {
	home := setupTestHome(t)

	settingsPath := filepath.Join(home, ".claude", "settings.json")
	writeJSON(t, settingsPath, map[string]interface{}{
		"permissions": map[string]interface{}{"allow": []string{"Bash(*)"}},
	})

	if err := removeHooks(".claude"); err != nil {
		t.Fatalf("removeHooks: %v", err)
	}

	settings := readJSON(t, settingsPath)
	if _, ok := settings["permissions"]; !ok {
		t.Error("permissions lost after removeHooks when no hooks existed")
	}
}

func TestRemoveHooks_PreservesNonTimoHooks(t *testing.T) {
	home := setupTestHome(t)

	settingsData := map[string]interface{}{
		"hooks": map[string]interface{}{
			"PreToolUse": []interface{}{
				map[string]interface{}{
					"command": "/usr/bin/timo notify --type claude-pre-tool",
				},
				map[string]interface{}{
					"command": "/usr/bin/my-custom-hook",
				},
			},
		},
	}
	settingsPath := filepath.Join(home, ".claude", "settings.json")
	writeJSON(t, settingsPath, settingsData)

	if err := removeHooks(".claude"); err != nil {
		t.Fatalf("removeHooks: %v", err)
	}

	settings := readJSON(t, settingsPath)
	hooks, ok := settings["hooks"].(map[string]interface{})
	if !ok {
		t.Fatal("hooks section lost entirely")
	}

	preToolHooks, ok := hooks["PreToolUse"].([]interface{})
	if !ok {
		t.Fatal("PreToolUse lost entirely")
	}

	if len(preToolHooks) != 1 {
		t.Errorf("expected 1 hook remaining, got %d", len(preToolHooks))
	}
}

func TestRemoveHooks_ClaudeAndReasonix_CleanIndependently(t *testing.T) {
	home := setupTestHome(t)

	settingsData := map[string]interface{}{
		"hooks": map[string]interface{}{
			"PreToolUse": []interface{}{
				map[string]interface{}{
					"command": "/usr/bin/timo notify --type reasonix-pre-tool",
				},
			},
		},
	}
	claudePath := filepath.Join(home, ".claude", "settings.json")
	reasonixPath := filepath.Join(home, ".reasonix", "settings.json")
	writeJSON(t, claudePath, settingsData)
	writeJSON(t, reasonixPath, settingsData)

	// Remove only claude hooks
	if err := removeHooks(".claude"); err != nil {
		t.Fatalf("removeHooks claude: %v", err)
	}

	claudeSettings := readJSON(t, claudePath)
	if _, ok := claudeSettings["hooks"]; ok {
		t.Error("claude hooks still present after removeHooks")
	}

	reasonixSettings := readJSON(t, reasonixPath)
	if _, ok := reasonixSettings["hooks"]; !ok {
		t.Error("reasonix hooks lost when removing claude hooks")
	}
}
