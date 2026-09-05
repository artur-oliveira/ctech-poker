package auth

import (
	"fmt"
	"os/exec"
	"runtime"
)

// OpenBrowser launches the system browser at url without waiting for it to
// exit — a native app launch is fire-and-forget, and LoginPKCE relies on that
// (it also calls this in the background, but a blocking Start would still
// tie up that goroutine for the browser's whole lifetime otherwise). Falls
// back to printing the URL when no launcher is found or the launch fails.
func OpenBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	if err := cmd.Start(); err != nil {
		fmt.Printf("Open this URL to log in:\n%s\n", url)
		return err
	}
	return nil
}
