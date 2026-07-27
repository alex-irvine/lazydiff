package pr

import "testing"

func TestOpenerCommandLinux(t *testing.T) {
	name, args, err := openerCommand("linux", "https://example.com")
	if err != nil || name != "xdg-open" || len(args) != 1 || args[0] != "https://example.com" {
		t.Fatalf("name=%q args=%v err=%v", name, args, err)
	}
}

func TestOpenerCommandDarwin(t *testing.T) {
	name, args, err := openerCommand("darwin", "https://example.com")
	if err != nil || name != "open" || len(args) != 1 || args[0] != "https://example.com" {
		t.Fatalf("name=%q args=%v err=%v", name, args, err)
	}
}

func TestOpenerCommandWindowsAvoidsShell(t *testing.T) {
	name, args, err := openerCommand("windows", "https://example.com?a=1&b=2")
	if err != nil || name != "rundll32" || len(args) != 2 || args[0] != "url.dll,FileProtocolHandler" || args[1] != "https://example.com?a=1&b=2" {
		t.Fatalf("name=%q args=%v err=%v", name, args, err)
	}
}

func TestOpenerCommandUnsupportedOS(t *testing.T) {
	if _, _, err := openerCommand("plan9", "https://example.com"); err == nil {
		t.Fatal("expected error for unsupported OS")
	}
}
