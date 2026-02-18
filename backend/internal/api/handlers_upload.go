package api

import (
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/cpay-dev/backend/internal/auth"
)

const maxAvatarSize = 5 << 20 // 5MB

var allowedContentTypes = map[string]bool{
	"image/png":  true,
	"image/jpeg": true,
	"image/webp": true,
}

func (h *handlers) uploadAvatar(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())

	r.Body = http.MaxBytesReader(w, r.Body, maxAvatarSize)
	if err := r.ParseMultipartForm(maxAvatarSize); err != nil {
		writeError(w, http.StatusBadRequest, "file too large (max 5MB)")
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "missing file field")
		return
	}
	defer file.Close()

	contentType := header.Header.Get("Content-Type")
	if !allowedContentTypes[contentType] {
		writeError(w, http.StatusBadRequest, "invalid file type (allowed: png, jpeg, webp)")
		return
	}

	ext := filepath.Ext(header.Filename)
	if ext == "" {
		switch contentType {
		case "image/png":
			ext = ".png"
		case "image/jpeg":
			ext = ".jpg"
		case "image/webp":
			ext = ".webp"
		}
	}
	filename := fmt.Sprintf("avatar%s", strings.ToLower(ext))

	url, err := h.storage.UploadAvatar(r.Context(), userID, filename, file, header.Size, contentType)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to upload file")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"url": url})
}
