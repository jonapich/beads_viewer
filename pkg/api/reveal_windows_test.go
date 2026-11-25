//go:build windows

package api

import "testing"

func TestExplorerCommandLine(t *testing.T) {
	got, err := explorerCommandLine(`C:\My Shots\a, b.png`)
	if err != nil || got != `explorer.exe /select,"C:\My Shots\a, b.png"` {
		t.Fatalf("command line = %q, %v", got, err)
	}
	for _, bad := range []string{"", `C:\a"b.png`, "C:\\a\nb.png", "C:\\a\x00.png"} {
		if _, err := explorerCommandLine(bad); err == nil {
			t.Errorf("%q must be refused", bad)
		}
	}
}
