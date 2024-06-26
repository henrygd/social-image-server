package templates

import (
	"log"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"

	"github.com/henrygd/social-image-server/internal/global"
)

func TempServer(templateName string) (server *http.Server, serverURL string, err error) {
	slog.Debug("Starting temp server", "template", templateName)
	// Serve static files from template directory
	fs := http.FileServer(http.Dir(filepath.Join(global.TemplateDir, templateName)))
	// Create a new ServeMux and handle root path
	router := http.NewServeMux()
	router.Handle("/", fs)

	// Create a listener on a random port
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		return nil, "", err
	}

	server = &http.Server{
		Addr:    listener.Addr().String(),
		Handler: router,
	}

	// Start the server in a new goroutine
	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server ListenAndServe(): %v", err)
		}
	}()

	return server, "http://" + server.Addr, nil
}

func IsValid(templateName string) bool {
	entries, _ := os.ReadDir(global.TemplateDir)
	for _, entry := range entries {
		if entry.Name() == templateName {
			return true
		}
	}
	return false
}
