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

const fishWrapper = `if type -q gg
  function gg
    if test (count $argv) -ge 1; and test $argv[1] = jump
      set -l target (command gg jump $argv[2..-1])
      or return $status
      cd $target
    else
      command gg $argv
    end
  end
  command gg completion fish | source
end
`

func posixWrapper(shell string) string {
	return fmt.Sprintf(`if command -v gg >/dev/null 2>&1; then
  gg() {
    if [ "$1" = "jump" ]; then
      shift
      local __gg_target
      __gg_target="$(command gg jump "$@")" || return $?
      cd "$__gg_target"
    else
      command gg "$@"
    fi
  }
  eval "$(command gg completion %s)"
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

func shellConfigPath(shell string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	switch shell {
	case "fish":
		return filepath.Join(home, ".config", "fish", "config.fish"), nil
	case "zsh":
		return filepath.Join(home, ".zshrc"), nil
	case "bash":
		return filepath.Join(home, ".bashrc"), nil
	}
	return "", fmt.Errorf("unsupported shell %q", shell)
}

func wrapperFor(shell string) (string, error) {
	switch shell {
	case "fish":
		return fishWrapper, nil
	case "bash", "zsh":
		return posixWrapper(shell), nil
	}
	return "", fmt.Errorf("unsupported shell %q", shell)
}

func installShellWrapper() (ShellInstallResult, error) {
	res := ShellInstallResult{}
	shell := detectShell()
	if shell == "" {
		res.Action = ShellSkipped
		return res, nil
	}
	res.Shell = shell

	cfg, err := shellConfigPath(shell)
	if err != nil {
		return res, err
	}
	res.Path = cfg

	wrapper, err := wrapperFor(shell)
	if err != nil {
		return res, err
	}
	full := shellMarkerBegin + "\n" + wrapper + shellMarkerEnd + "\n"

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
