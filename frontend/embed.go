// Package frontend exposes the dependency-free browser UI as embedded files.
package frontend

import (
	"embed"
	"net/http"
)

//go:embed index.html styles.css app.js
var files embed.FS

// Handler serves the frontend without depending on the process working
// directory. This keeps local, compiled, and Replit runs identical.
func Handler() http.Handler {
	return http.FileServer(http.FS(files))
}
