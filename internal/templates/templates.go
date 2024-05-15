package templates

import (
	"log"
	"net"
	"net/http"
	"path/filepath"

	"github.com/henrygd/social-image-server/internal/global"
)

func TempServer(templateName string) (*http.Server, net.Listener, error) {
	// Serve static files from template directory
	fs := http.FileServer(http.Dir(filepath.Join(global.TemplateDir, templateName)))
	// Create a new ServeMux and handle root path
	router := http.NewServeMux()
	router.Handle("/", fs)

	// Create a listener on a random port
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		// slog.Error("Creating template listener", "error", err)
		return nil, nil, err
	}

	// Get the actual address (with the assigned port)
	// addr := listener.Addr().String()
	// slog.Debug("Server is listening on %s\n", addr)

	server := &http.Server{
		Addr:    listener.Addr().String(),
		Handler: router,
	}

	// Start the server in a new goroutine
	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server ListenAndServe(): %v", err)
		}
	}()

	return server, listener, nil

}
