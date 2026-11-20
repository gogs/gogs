package mimex

import (
	"net/http"
	"strings"
)

// IsText returns true if the content is plain text or empty.
func IsText(data []byte) bool {
	if len(data) == 0 {
		return true
	}
	return strings.Contains(http.DetectContentType(data), "text/")
}

// IsImage returns true if the content is an image.
func IsImage(data []byte) bool {
	return strings.Contains(http.DetectContentType(data), "image/")
}

// IsPDF returns true if the content is a PDF document.
func IsPDF(data []byte) bool {
	return strings.Contains(http.DetectContentType(data), "application/pdf")
}

// IsVideo returns true if the content is a video.
func IsVideo(data []byte) bool {
	return strings.Contains(http.DetectContentType(data), "video/")
}
