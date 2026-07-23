package admin

import (
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// flyersSubdir keeps event flyers separate from gallery photos inside the
// uploads directory.
const flyersSubdir = "flyers"

var allowedImageExt = map[string]bool{
	".jpg": true, ".jpeg": true, ".png": true, ".gif": true, ".webp": true,
}

// errNotAnImage means the upload failed the extension or content check.
var errNotAnImage = errors.New("not an image")

// saveImage writes an uploaded image into dir under a server-generated
// name and returns that name. The original filename is never used, so
// there's no path-traversal surface.
func saveImage(fileHeader *multipart.FileHeader, dir string) (string, error) {
	ext := strings.ToLower(filepath.Ext(fileHeader.Filename))
	if !allowedImageExt[ext] {
		return "", errNotAnImage
	}

	src, err := fileHeader.Open()
	if err != nil {
		return "", err
	}
	defer src.Close()

	// Extension whitelist plus a content sniff — belt and suspenders
	// against a renamed non-image ending up on a publicly served path.
	buf := make([]byte, 512)
	n, _ := io.ReadFull(src, buf)
	if !strings.HasPrefix(http.DetectContentType(buf[:n]), "image/") {
		return "", errNotAnImage
	}
	if _, err := src.Seek(0, io.SeekStart); err != nil {
		return "", err
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}

	filename := fmt.Sprintf("%d%s", time.Now().UnixNano(), ext)
	dst, err := os.Create(filepath.Join(dir, filename))
	if err != nil {
		return "", err
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return "", err
	}
	return filename, dst.Close()
}
