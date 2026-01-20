package llm

import (
	"mime"
	"strings"
)

// MimeType is an IANA media type string (e.g. "image/png").
type MimeType string

const (
	MimeTypePDF  MimeType = "application/pdf"
	MimeTypeJPEG MimeType = "image/jpeg"
	MimeTypePNG  MimeType = "image/png"
	MimeTypeGIF  MimeType = "image/gif"
	MimeTypeWebP MimeType = "image/webp"
	MimeTypeMP3  MimeType = "audio/mpeg"
	MimeTypeWAV  MimeType = "audio/wav"
)

// FileKind groups MIME types into broad media categories.
type FileKind string

const (
	FileKindUnknown FileKind = ""
	FileKindImage   FileKind = "image"
	FileKindPDF     FileKind = "pdf"
	FileKindAudio   FileKind = "audio"
	FileKindVideo   FileKind = "video"
)

// NormalizeMimeType trims, lowercases, and removes parameters from a MIME type.
// Returns empty string if the value is empty or cannot be parsed as a valid MIME type.
func NormalizeMimeType(value string) MimeType {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return ""
	}
	mediaType, _, err := mime.ParseMediaType(value)
	if err != nil || strings.TrimSpace(mediaType) == "" {
		// Return empty to signal invalid input - callers check for empty and fail fast.
		return ""
	}
	return MimeType(strings.ToLower(mediaType))
}

// FileKindFromMimeType infers a FileKind from the MIME type.
func FileKindFromMimeType(mimeType MimeType) FileKind {
	base := string(NormalizeMimeType(string(mimeType)))
	switch {
	case strings.HasPrefix(base, "image/"):
		return FileKindImage
	case base == string(MimeTypePDF):
		return FileKindPDF
	case strings.HasPrefix(base, "audio/"):
		return FileKindAudio
	case strings.HasPrefix(base, "video/"):
		return FileKindVideo
	default:
		return FileKindUnknown
	}
}
