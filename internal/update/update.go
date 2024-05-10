package update

import (
	"log"
	"log/slog"
	"os"
	"strings"

	"github.com/blang/semver"
	"github.com/rhysd/go-github-selfupdate/selfupdate"
)

func Run(curVersion string) {
	var latest *selfupdate.Release
	var found bool
	var err error
	currentVersion := semver.MustParse(curVersion)
	slog.Info("Social Image Server", "v", currentVersion)
	slog.Info("Checking for updates...")
	latest, found, err = selfupdate.DetectLatest("henrygd/social-image-server")

	if err != nil {
		slog.Error("Error checking for updates", "err", err)
	}

	if !found {
		slog.Error("No updates found")
		os.Exit(1)
	}

	slog.Info("Latest version", "v", latest.Version)

	if latest.Version.LTE(currentVersion) {
		slog.Info("You are up to date")
		return
	}

	var binaryPath string
	log.Printf("Updating from %s to %s...", currentVersion, latest.Version)
	binaryPath, err = os.Executable()
	if err != nil {
		slog.Error("Error getting binary path", "error", err)
		os.Exit(1)
	}
	err = selfupdate.UpdateTo(latest.AssetURL, binaryPath)
	if err != nil {
		slog.Error("Please try rerunning with sudo", "err", err)
		os.Exit(1)
	}
	log.Printf("Successfully updated: %s -> %s\n\n%s", currentVersion, latest.Version, strings.TrimSpace(latest.ReleaseNotes))
}
