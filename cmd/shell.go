package cmd

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	shellMarkerBegin = "# >>> gg shell setup >>>"
	shellMarkerEnd   = "# <<< gg shell setup <<<"
)

const (
	ShellInstalled = "installed"
	ShellUpdated   = "updated"
	ShellUnchanged = "unchanged"
	ShellSkipped   = "skipped"
)

type ShellInstallResult struct {
	Action string
	Shell  string
	Path   string
}

const fishFunctionAutoload = `function gg
  if test (count $argv) -ge 1; and contains -- $argv[1] jump clone new
    set -l __gg_sub $argv[1]
    set -l target (command gg $__gg_sub $argv[2..-1])
    or return $status
    cd $target
  else
    command gg $argv
  end
end
`

const fishConfdSnippet = `if type -q gg
    command gg refresh --if-stale </dev/null >/dev/null 2>&1 &
    disown 2>/dev/null
end
`

func posixWrapper(shell string) string {
	return fmt.Sprintf(`if command -v gg >/dev/null 2>&1; then
  gg() {
    case "$1" in
      jump|clone|new)
        local __gg_sub="$1"
        shift
        local __gg_target
        __gg_target="$(command gg "$__gg_sub" "$@")" || return $?
        cd "$__gg_target"
        ;;
      *)
        command gg "$@"
        ;;
    esac
  }
  eval "$(command gg completion %s)"
  command gg refresh --if-stale </dev/null >/dev/null 2>&1 &
  disown 2>/dev/null
fi
`, shell)
}

func detectShell() string {
	base := filepath.Base(os.Getenv("SHELL"))
	switch {
	case strings.HasSuffix(base, "fish"):
		return "fish"
	case strings.HasSuffix(base, "zsh"):
		return "zsh"
	case strings.HasSuffix(base, "bash"):
		return "bash"
	}
	return ""
}

func installShellWrapper() (ShellInstallResult, error) {
	res := ShellInstallResult{}
	shell := detectShell()
	if shell == "" {
		res.Action = ShellSkipped
		return res, nil
	}
	res.Shell = shell

	if shell == "fish" {
		return installFishAutoload()
	}
	return installPosixWrapper(shell)
}

func installFishAutoload() (ShellInstallResult, error) {
	res := ShellInstallResult{Shell: "fish"}
	home, err := os.UserHomeDir()
	if err != nil {
		return res, err
	}
	fnPath := filepath.Join(home, ".config", "fish", "functions", "gg.fish")
	compPath := filepath.Join(home, ".config", "fish", "completions", "gg.fish")
	confdPath := filepath.Join(home, ".config", "fish", "conf.d", "gg.fish")
	res.Path = fnPath

	var compBuf bytes.Buffer
	if err := rootCmd.GenFishCompletion(&compBuf, true); err != nil {
		return res, fmt.Errorf("generate fish completion: %w", err)
	}

	fnAction, err := writeFishFile(fnPath, []byte(fishFunctionAutoload))
	if err != nil {
		return res, err
	}
	compAction, err := writeFishFile(compPath, compBuf.Bytes())
	if err != nil {
		return res, err
	}
	confdAction, err := writeFishFile(confdPath, []byte(fishConfdSnippet))
	if err != nil {
		return res, err
	}

	cfgPath := filepath.Join(home, ".config", "fish", "config.fish")
	cleanupOldShellBlock(cfgPath)

	switch {
	case fnAction == ShellInstalled || compAction == ShellInstalled || confdAction == ShellInstalled:
		res.Action = ShellInstalled
	case fnAction == ShellUpdated || compAction == ShellUpdated || confdAction == ShellUpdated:
		res.Action = ShellUpdated
	default:
		res.Action = ShellUnchanged
	}
	return res, nil
}

func writeFishFile(path string, content []byte) (string, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	existing, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	if bytes.Equal(existing, content) {
		return ShellUnchanged, nil
	}
	action := ShellUpdated
	if os.IsNotExist(err) || len(existing) == 0 {
		action = ShellInstalled
	}
	return action, os.WriteFile(path, content, 0o644)
}

func cleanupOldShellBlock(cfgPath string) {
	existing, err := os.ReadFile(cfgPath)
	if err != nil {
		return
	}
	if !bytes.Contains(existing, []byte(shellMarkerBegin)) {
		return
	}
	cleaned := stripShellBlock(existing)
	if !bytes.Equal(cleaned, existing) {
		_ = os.WriteFile(cfgPath, cleaned, 0o644)
	}
}

func stripShellBlock(existing []byte) []byte {
	begin := []byte(shellMarkerBegin)
	end := []byte(shellMarkerEnd)
	bi := bytes.Index(existing, begin)
	if bi < 0 {
		return existing
	}
	rest := existing[bi:]
	ei := bytes.Index(rest, end)
	if ei < 0 {
		return existing
	}
	afterEnd := bi + ei + len(end)
	if afterEnd < len(existing) && existing[afterEnd] == '\n' {
		afterEnd++
	}
	var buf bytes.Buffer
	buf.Write(existing[:bi])
	buf.Write(existing[afterEnd:])
	return buf.Bytes()
}

func installPosixWrapper(shell string) (ShellInstallResult, error) {
	res := ShellInstallResult{Shell: shell}
	home, err := os.UserHomeDir()
	if err != nil {
		return res, err
	}
	var cfg string
	switch shell {
	case "zsh":
		cfg = filepath.Join(home, ".zshrc")
	case "bash":
		cfg = filepath.Join(home, ".bashrc")
	default:
		return res, fmt.Errorf("unsupported shell %q", shell)
	}
	res.Path = cfg

	full := shellMarkerBegin + "\n" + posixWrapper(shell) + shellMarkerEnd + "\n"

	if err := os.MkdirAll(filepath.Dir(cfg), 0o755); err != nil {
		return res, err
	}

	existing, err := os.ReadFile(cfg)
	if err != nil && !os.IsNotExist(err) {
		return res, err
	}

	if bytes.Contains(existing, []byte(shellMarkerBegin)) {
		next := replaceShellBlock(existing, full)
		if bytes.Equal(next, existing) {
			res.Action = ShellUnchanged
			return res, nil
		}
		if err := os.WriteFile(cfg, next, 0o644); err != nil {
			return res, err
		}
		res.Action = ShellUpdated
		return res, nil
	}

	var buf bytes.Buffer
	buf.Write(existing)
	if len(existing) > 0 && !bytes.HasSuffix(existing, []byte("\n")) {
		buf.WriteByte('\n')
	}
	buf.WriteString(full)
	if err := os.WriteFile(cfg, buf.Bytes(), 0o644); err != nil {
		return res, err
	}
	res.Action = ShellInstalled
	return res, nil
}

func replaceShellBlock(existing []byte, full string) []byte {
	begin := []byte(shellMarkerBegin)
	end := []byte(shellMarkerEnd)
	bi := bytes.Index(existing, begin)
	if bi < 0 {
		return existing
	}
	rest := existing[bi:]
	ei := bytes.Index(rest, end)
	if ei < 0 {
		return existing
	}
	afterEnd := bi + ei + len(end)
	if afterEnd < len(existing) && existing[afterEnd] == '\n' {
		afterEnd++
	}
	var buf bytes.Buffer
	buf.Write(existing[:bi])
	buf.WriteString(full)
	buf.Write(existing[afterEnd:])
	return buf.Bytes()
}
