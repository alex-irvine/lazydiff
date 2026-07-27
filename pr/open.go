package pr

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
)

// Opener opens a URL in the user's default browser.
type Opener interface {
	Open(ctx context.Context, rawURL string) error
}

type osOpener struct{ goos string }

// NewOpener returns the real, OS-appropriate Opener.
func NewOpener() Opener {
	return osOpener{goos: runtime.GOOS}
}

func (o osOpener) Open(ctx context.Context, rawURL string) error {
	name, args, err := openerCommand(o.goos, rawURL)
	if err != nil {
		return err
	}
	if err := exec.CommandContext(ctx, name, args...).Start(); err != nil {
		return fmt.Errorf("open %q: %w", rawURL, err)
	}
	return nil
}

func openerCommand(goos, rawURL string) (string, []string, error) {
	switch goos {
	case "linux":
		return "xdg-open", []string{rawURL}, nil
	case "darwin":
		return "open", []string{rawURL}, nil
	case "windows":
		// Invoked directly (not via `cmd /c start`), which would treat the
		// URL's `&` query-param separators as command separators and
		// truncate it.
		return "rundll32", []string{"url.dll,FileProtocolHandler", rawURL}, nil
	default:
		return "", nil, fmt.Errorf("unsupported OS %q for opening a browser", goos)
	}
}
