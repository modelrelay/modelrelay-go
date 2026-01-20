package llm

import "encoding/base64"

// FilePart wraps file content into a content part.
func FilePart(file FileContent) ContentPart {
	return ContentPart{Type: ContentPartTypeFile, File: &file}
}

// FilePartFromBytes encodes raw bytes as base64 file content.
func FilePartFromBytes(data []byte, mimeType MimeType, filename string) ContentPart {
	return FilePart(FileContent{
		DataBase64: base64.StdEncoding.EncodeToString(data),
		MimeType:   mimeType,
		Filename:   filename,
		SizeBytes:  int64(len(data)),
	})
}
